package api

import (
	"DarkCS/internal/config"
	"DarkCS/internal/http-server/handlers/assistant"
	"DarkCS/internal/http-server/handlers/errors"
	"DarkCS/internal/http-server/handlers/product"
	"DarkCS/internal/http-server/handlers/response"
	"DarkCS/internal/http-server/handlers/service"
	"DarkCS/internal/http-server/handlers/user"
	"DarkCS/internal/http-server/middleware/authenticate"
	"DarkCS/internal/http-server/middleware/timeout"
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net"
	"net/http"
)

type Server struct {
	conf       *config.Config
	httpServer *http.Server
	log        *slog.Logger
}

type Handler interface {
	authenticate.Authenticate
	service.Service
	product.Core
	response.Core
	user.Core
	assistant.Core
}

func New(conf *config.Config, log *slog.Logger, handler Handler) error {

	server := Server{
		conf: conf,
		log:  log.With(sl.Module("api.server")),
	}

	router := chi.NewRouter()
	router.Use(timeout.Timeout(5))
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(render.SetContentType(render.ContentTypeJSON))
	router.Use(authenticate.New(log, handler))

	router.NotFound(errors.NotFound(log))
	router.MethodNotAllowed(errors.NotAllowed(log))

	router.Route("/api/v1", func(v1 chi.Router) {
		v1.Route("/products", func(r chi.Router) {
			r.Post("/info", product.ProductsInfo(log, handler))
		})
		v1.Route("/response", func(r chi.Router) {
			r.Post("/", response.ComposeResponse(log, handler))
		})
		v1.Route("/user", func(r chi.Router) {
			r.Get("/", user.GetUser(log, handler))
			r.Post("/block", user.BlockUser(log, handler))
		})
		v1.Route("/assistant", func(r chi.Router) {
			r.Get("/attach", assistant.AttachFile(log, handler))
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
