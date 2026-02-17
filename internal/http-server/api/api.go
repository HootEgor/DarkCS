package api

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	"DarkCS/bot/insta"
	"DarkCS/bot/whatsapp"
	"DarkCS/internal/config"
	"DarkCS/internal/http-server/handlers/assistant"
	"DarkCS/internal/http-server/handlers/crm"
	"DarkCS/internal/http-server/handlers/errors"
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
	"DarkCS/internal/http-server/middleware/timeout"
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
}

func New(conf *config.Config, log *slog.Logger, handler Handler, opts ...Option) error {

	server := Server{
		conf: conf,
		log:  log.With(sl.Module("api.server")),
	}

	for _, opt := range opts {
		opt(&server)
	}

	router := chi.NewRouter()
	router.Use(timeout.Timeout(5))
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(corsMiddleware)
	router.Use(render.SetContentType(render.ContentTypeJSON))

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

		// Authenticated routes
		v1.Group(func(auth chi.Router) {
			auth.Use(authenticate.New(log, handler))
			auth.Route("/products", func(r chi.Router) {
				r.Post("/info", product.ProductsInfo(log, handler))
			})
			auth.Route("/response", func(r chi.Router) {
				r.Post("/", response.ComposeResponse(log, handler))
			})
			auth.Route("/user", func(r chi.Router) {
				r.Get("/", user.GetUser(log, handler))
				r.Post("/create", user.CreateUser(log, handler))
				r.Post("/block", user.BlockUser(log, handler))
				r.Post("/promo", user.GetUserPromoAccess(log, handler))
				r.Post("/activate", user.ActivateUserPromo(log, handler))
				r.Post("/close", user.CloseUserPromo(log, handler))
				r.Post("/phone", user.CheckPhone(log, handler))
				r.Get("/reset_conv", user.ResetConversation(log, handler))
				r.Post("/import-telegram", user.ImportTelegram(log, handler))
			})
			auth.Route("/assistant", func(r chi.Router) {
				r.Get("/attach", assistant.AttachFile(log, handler))
				r.Post("/update", assistant.Update(log, handler))
				r.Get("/all", assistant.GetAllAssistants(log, handler))
			})
			auth.Route("/zoho", func(r chi.Router) {
				r.Post("/order_products", zoho.GetOrderProducts(log, handler))
			})
			auth.Route("/promo", func(r chi.Router) {
				r.Get("/get", promo.GetActivePromoCodes(log, handler))
				r.Post("/generate", promo.GeneratePromoCodes(log, handler))
			})
			auth.Route("/smart", func(r chi.Router) {
				r.Post("/send", smart.SendMsg(log, handler))
			})
			auth.Route("/key", func(r chi.Router) {
				r.Post("/new", key.Generate(log, handler))
			})
			auth.Route("/qr", func(r chi.Router) {
				r.Post("/follow", qr_stat.FollowQr(log, handler))
				r.Post("/stat", qr_stat.GetStat(log, handler))
			})
			auth.Route("/school", func(r chi.Router) {
				r.Post("/add", school.AddSchools(log, handler))
				r.Get("/list", school.ListSchools(log, handler))
				r.Post("/status", school.SetStatus(log, handler))
			})
			auth.Post("/mcp", mcp.Handler(log, handler))
			auth.Route("/crm", func(r chi.Router) {
				r.Get("/chats", crm.GetChats(log, handler))
				r.Get("/chats/{platform}/{user_id}/messages", crm.GetMessages(log, handler))
				r.Post("/chats/{platform}/{user_id}/send", crm.SendMessage(log, handler))
				r.Post("/chats/{platform}/{user_id}/send-file", crm.SendFile(log, handler))
			})
		})
	})

	httpLog := slog.NewLogLogger(log.Handler(), slog.LevelError)
	server.httpServer = &http.Server{
		Handler:  router,
		ErrorLog: httpLog,
	}

	serverAddress := fmt.Sprintf("%s:%s", conf.Listen.BindIP, conf.Listen.Port)
	listener, err := net.Listen("tcp", serverAddress)
	if err != nil {
		return err
	}

	server.log.Info("starting api server", slog.String("address", serverAddress))

	return server.httpServer.Serve(listener)
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
