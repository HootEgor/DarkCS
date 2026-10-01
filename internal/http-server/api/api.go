package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	"DarkCS/bot/insta"
	"DarkCS/bot/whatsapp"
	"DarkCS/entity"
	"DarkCS/internal/config"
	"DarkCS/internal/http-server/handlers/assistant"
	"DarkCS/internal/http-server/handlers/crm"
	"DarkCS/internal/http-server/handlers/errors"
	"DarkCS/internal/http-server/handlers/health"
	"DarkCS/internal/http-server/handlers/instagram"
	"DarkCS/internal/http-server/handlers/key"
	"DarkCS/internal/http-server/handlers/mcp"
	"DarkCS/internal/http-server/handlers/product"
	"DarkCS/internal/http-server/handlers/promo"
	"DarkCS/internal/http-server/handlers/qr-stat"
	"DarkCS/internal/http-server/handlers/response"
	"DarkCS/internal/http-server/handlers/school"
	"DarkCS/internal/http-server/handlers/service"
	"DarkCS/internal/http-server/handlers/smart"
	"DarkCS/internal/http-server/handlers/user"
	wa "DarkCS/internal/http-server/handlers/whatsapp"
	"DarkCS/internal/http-server/handlers/zoho"
	"DarkCS/internal/http-server/middleware/authenticate"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/ws"
)

type Server struct {
	conf        *config.Config
	httpServer  *http.Server
	log         *slog.Logger
	instaBot    *insta.InstaBot
	whatsappBot *whatsapp.WhatsAppBot
	wsHub       *ws.Hub
	wsAuth      ws.Authenticator
}

// Option is a functional option for configuring the server
type Option func(*Server)

// WithInstaBot sets the Instagram bot for the server
func WithInstaBot(bot *insta.InstaBot) Option {
	return func(s *Server) {
		s.instaBot = bot
	}
}

// WithWhatsAppBot sets the WhatsApp bot for the server
func WithWhatsAppBot(bot *whatsapp.WhatsAppBot) Option {
	return func(s *Server) {
		s.whatsappBot = bot
	}
}

// WithWsHub sets the WebSocket hub and authenticator for the server
func WithWsHub(hub *ws.Hub, auth ws.Authenticator) Option {
	return func(s *Server) {
		s.wsHub = hub
		s.wsAuth = auth
	}
}

type Handler interface {
	authenticate.Authenticate
	service.Service
	product.Core
	response.Core
	user.Core
	assistant.Core
	zoho.Core
	promo.Core
	smart.Core
	key.Core
	qr_stat.Core
	mcp.Core
	school.Core
	crm.Core
	health.Core
	SetPublicURL(url string)
}

// Server timeouts. ReadHeaderTimeout stops slow-header (slowloris) connections; Read and
// Write are generous because AI answers can take a minute or more and CRM uploads and
// GridFS downloads stream large bodies. WebSocket connections set their own deadlines
// after the upgrade.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 2 * time.Minute
	writeTimeout      = 5 * time.Minute
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 30 * time.Second
)

// New builds the router and serves until ctx is cancelled, then shuts down gracefully:
// it stops accepting connections and waits up to shutdownTimeout for in-flight requests.
func New(ctx context.Context, conf *config.Config, log *slog.Logger, handler Handler, opts ...Option) error {

	server := Server{
		conf: conf,
		log:  log.With(sl.Module("api.server")),
	}

	for _, opt := range opts {
		opt(&server)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(corsMiddleware)
	router.Use(render.SetContentType(render.ContentTypeJSON))
	if conf.Listen.PublicURL != "" {
		handler.SetPublicURL(strings.TrimRight(conf.Listen.PublicURL, "/"))
	} else {
		log.Warn("listen.public_url not set; detecting it from the first request's Host header")
		router.Use(detectPublicURL(handler, log))
	}

	router.Get("/healthz", health.Handler(handler, time.Now()))

	router.NotFound(errors.NotFound(log))
	router.MethodNotAllowed(errors.NotAllowed(log))

	// Webhook routes (no auth required for Meta verification)
	router.Route("/webhook", func(r chi.Router) {
		if server.instaBot != nil {
			r.Get("/instagram", instagram.WebhookVerify(log, server.instaBot))
			r.Post("/instagram", instagram.WebhookHandler(log, server.instaBot))
		}
		if server.whatsappBot != nil {
			r.Get("/whatsapp", wa.WebhookVerify(log, server.whatsappBot))
			r.Post("/whatsapp", wa.WebhookHandler(log, server.whatsappBot))
		}
	})

	// API v1 routes
	router.Route("/api/v1", func(v1 chi.Router) {
		// WebSocket endpoint (handles its own auth via query param, no middleware)
		if server.wsHub != nil && server.wsAuth != nil {
			v1.Get("/crm/ws", func(w http.ResponseWriter, r *http.Request) {
				ws.ServeWs(server.wsHub, server.wsAuth, log, w, r)
			})
		}

		// File download endpoint — authenticated via HMAC-signed URL
		v1.Get("/crm/files/{file_id}", crm.DownloadFile(log, handler))

		// Authenticated routes. Each endpoint declares which key scopes may call it
		// (admin keys pass everywhere; see authenticate.RequireScope for legacy keys).
		integration := authenticate.RequireScope(log, "integration", entity.ScopeIntegration)
		admin := authenticate.RequireScope(log, "admin")
		crmOnly := authenticate.RequireScope(log, "crm", entity.ScopeCRM)
		mcpOnly := authenticate.RequireScope(log, "mcp", entity.ScopeMCP)

		v1.Group(func(auth chi.Router) {
			auth.Use(authenticate.New(log, handler))
			auth.Route("/products", func(r chi.Router) {
				r.With(integration).Post("/info", product.ProductsInfo(log, handler))
			})
			auth.Route("/response", func(r chi.Router) {
				r.With(integration).Post("/", response.ComposeResponse(log, handler))
			})
			auth.Route("/user", func(r chi.Router) {
				r.With(integration).Get("/", user.GetUser(log, handler))
				r.With(integration).Post("/create", user.CreateUser(log, handler))
				r.With(admin).Post("/block", user.BlockUser(log, handler))
				r.With(integration).Post("/promo", user.GetUserPromoAccess(log, handler))
				r.With(integration).Post("/activate", user.ActivateUserPromo(log, handler))
				r.With(admin).Post("/close", user.CloseUserPromo(log, handler))
				r.With(integration).Post("/phone", user.CheckPhone(log, handler))
				r.With(admin).Get("/reset_conv", user.ResetConversation(log, handler))
				r.With(admin).Post("/import-telegram", user.ImportTelegram(log, handler))
			})
			auth.Route("/assistant", func(r chi.Router) {
				r.With(admin).Get("/attach", assistant.AttachFile(log, handler))
				r.With(admin).Post("/update", assistant.Update(log, handler))
				r.With(admin).Get("/all", assistant.GetAllAssistants(log, handler))
			})
			auth.Route("/zoho", func(r chi.Router) {
				r.With(integration).Post("/order_products", zoho.GetOrderProducts(log, handler))
			})
			auth.Route("/promo", func(r chi.Router) {
				r.With(admin).Get("/get", promo.GetActivePromoCodes(log, handler))
				r.With(admin).Post("/generate", promo.GeneratePromoCodes(log, handler))
			})
			auth.Route("/smart", func(r chi.Router) {
				r.With(integration).Post("/send", smart.SendMsg(log, handler))
			})
			auth.Route("/key", func(r chi.Router) {
				r.With(admin).Post("/new", key.Generate(log, handler))
			})
			auth.Route("/qr", func(r chi.Router) {
				r.With(integration).Post("/follow", qr_stat.FollowQr(log, handler))
				r.With(admin).Post("/stat", qr_stat.GetStat(log, handler))
			})
			auth.Route("/school", func(r chi.Router) {
				r.With(admin).Post("/add", school.AddSchools(log, handler))
				r.With(integration).Get("/list", school.ListSchools(log, handler))
				r.With(admin).Post("/status", school.SetStatus(log, handler))
			})
			auth.With(mcpOnly).Post("/mcp", mcp.Handler(log, handler))
			auth.Route("/crm", func(r chi.Router) {
				r.Use(crmOnly)
				r.Post("/ws-ticket", crm.IssueWsTicket(log, handler))
				r.Get("/chats", crm.GetChats(log, handler))
				r.Get("/chats/{platform}/{user_id}/messages", crm.GetMessages(log, handler))
				r.Post("/chats/{platform}/{user_id}/send", crm.SendMessage(log, handler))
				r.Post("/chats/{platform}/{user_id}/send-file", crm.SendFile(log, handler))
			})
		})
	})

	httpLog := slog.NewLogLogger(log.Handler(), slog.LevelError)
	server.httpServer = &http.Server{
		Handler:           router,
		ErrorLog:          httpLog,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverAddress := fmt.Sprintf("%s:%s", conf.Listen.BindIP, conf.Listen.Port)
	listener, err := net.Listen("tcp", serverAddress)
	if err != nil {
		return err
	}

	server.log.Info("starting api server", slog.String("address", serverAddress))

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.httpServer.Serve(listener) }()

	select {
	case err = <-serveErr:
		return err
	case <-ctx.Done():
	}

	server.log.Info("shutting down api server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err = server.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err = <-serveErr; err != nil && !stderrors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// detectPublicURL captures the server's public base URL from the first incoming
// request (using X-Forwarded-Proto / X-Forwarded-Host or the Host header) and
// stores it on the handler so file URLs can be built for external platforms.
func detectPublicURL(handler Handler, log *slog.Logger) func(http.Handler) http.Handler {
	var once sync.Once
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The deploy health check hits /healthz on localhost right after a restart;
			// it must not become the detected public URL.
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			once.Do(func() {
				scheme := "https"
				if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
					scheme = proto
				} else if r.TLS == nil {
					scheme = "http"
				}
				host := r.Header.Get("X-Forwarded-Host")
				if host == "" {
					host = r.Host
				}
				publicURL := scheme + "://" + host
				handler.SetPublicURL(publicURL)
				log.Info("public URL detected", slog.String("url", publicURL))
			})
			next.ServeHTTP(w, r)
		})
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
