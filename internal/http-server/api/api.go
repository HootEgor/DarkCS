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
	"DarkCS/internal/config"
	"DarkCS/internal/http-server/handlers/assistant"
	"DarkCS/internal/http-server/handlers/errors"
	"DarkCS/internal/http-server/handlers/instagram"
	"DarkCS/internal/http-server/handlers/key"
	"DarkCS/internal/http-server/handlers/mcp"
	"DarkCS/internal/http-server/handlers/product"
	"DarkCS/internal/http-server/handlers/promo"
	qr_stat "DarkCS/internal/http-server/handlers/qr-stat"
	"DarkCS/internal/http-server/handlers/response"
	"DarkCS/internal/http-server/handlers/school"
	"DarkCS/internal/http-server/handlers/service"
	"DarkCS/internal/http-server/handlers/smart"
	"DarkCS/internal/http-server/handlers/user"
	"DarkCS/internal/http-server/handlers/zoho"
	"DarkCS/internal/http-server/middleware/authenticate"
	"DarkCS/internal/http-server/middleware/timeout"
	"DarkCS/internal/lib/sl"
)

type Server struct {
	conf       *config.Config
	httpServer *http.Server
	log        *slog.Logger
	instaBot   *insta.InstaBot
}

// Option is a functional option for configuring the server
type Option func(*Server)

// WithInstaBot sets the Instagram bot for the server
func WithInstaBot(bot *insta.InstaBot) Option {
	return func(s *Server) {
		s.instaBot = bot
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
	router.Use(render.SetContentType(render.ContentTypeJSON))

	router.NotFound(errors.NotFound(log))
	router.MethodNotAllowed(errors.NotAllowed(log))

	// Instagram webhook routes (no auth required for Meta verification)
	if server.instaBot != nil {
		router.Route("/instagram", func(r chi.Router) {
			r.Get("/webhook", instagram.WebhookVerify(log, server.instaBot))
			r.Post("/webhook", instagram.WebhookHandler(log, server.instaBot))
		})
	}

	// Authenticated routes
	router.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(authenticate.New(log, handler))
		v1.Route("/products", func(r chi.Router) {
			r.Post("/info", product.ProductsInfo(log, handler))
		})
		v1.Route("/response", func(r chi.Router) {
			r.Post("/", response.ComposeResponse(log, handler))
		})
		v1.Route("/user", func(r chi.Router) {
			r.Get("/", user.GetUser(log, handler))
			r.Post("/create", user.CreateUser(log, handler))
			r.Post("/block", user.BlockUser(log, handler))
			r.Post("/promo", user.GetUserPromoAccess(log, handler))
			r.Post("/activate", user.ActivateUserPromo(log, handler))
			r.Post("/close", user.CloseUserPromo(log, handler))
			r.Post("/phone", user.CheckPhone(log, handler))
			r.Get("/reset_conv", user.ResetConversation(log, handler))
		})
		v1.Route("/assistant", func(r chi.Router) {
			r.Get("/attach", assistant.AttachFile(log, handler))
			r.Post("/update", assistant.Update(log, handler))
			r.Get("/all", assistant.GetAllAssistants(log, handler))
		})
		v1.Route("/zoho", func(r chi.Router) {
			r.Post("/order_products", zoho.GetOrderProducts(log, handler))
		})
		v1.Route("/promo", func(r chi.Router) {
			r.Get("/get", promo.GetActivePromoCodes(log, handler))
			r.Post("/generate", promo.GeneratePromoCodes(log, handler))
		})
		v1.Route("/smart", func(r chi.Router) {
			r.Post("/send", smart.SendMsg(log, handler))
		})
		v1.Route("/key", func(r chi.Router) {
			r.Post("/new", key.Generate(log, handler))
		})
		v1.Route("/qr", func(r chi.Router) {
			r.Post("/follow", qr_stat.FollowQr(log, handler))
			r.Post("/stat", qr_stat.GetStat(log, handler))
		})
		v1.Route("/school", func(r chi.Router) {
			r.Post("/add", school.AddSchools(log, handler))
			r.Get("/list", school.ListSchools(log, handler))
			r.Post("/status", school.SetStatus(log, handler))
		})
		v1.Post("/mcp", mcp.Handler(log, handler))
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
