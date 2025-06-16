package main

import (
	"DarkCS/impl/core"
	"DarkCS/internal/config"
	"DarkCS/internal/database"
	"DarkCS/internal/http-server/api"
	"DarkCS/internal/lib/logger"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/service/product"
	"flag"
	"log/slog"
)

func main() {

	configPath := flag.String("conf", "config.yml", "path to config file")
	logPath := flag.String("log", "/var/log/", "path to log file directory")
	flag.Parse()

	conf := config.MustLoad(*configPath)
	lg := logger.SetupLogger(conf.Env, *logPath)

	lg.Info("starting ocapi", slog.String("config", *configPath), slog.String("env", conf.Env))
	lg.Debug("debug messages enabled")

	handler := core.New(lg)
	handler.SetAuthKey(conf.Listen.ApiKey)

	db, err := repository.NewMongoClient(conf, lg)
	if err != nil {
		lg.With(
			sl.Err(err),
		).Error("mongo client")
	}
	if db != nil {
		handler.SetRepository(db)
		lg.With(
			slog.String("host", conf.Mongo.Host),
			slog.String("port", conf.Mongo.Port),
			slog.String("user", conf.Mongo.User),
			slog.String("database", conf.Mongo.Database),
		).Info("mongo client initialized")
	}

	ps := product.NewProductService(conf, lg)
	if ps != nil {
		handler.SetProductService(ps)
		lg.With(
			slog.String("login", conf.ProdService.Login),
			slog.String("url", conf.ProdService.BaseURL),
		).Info("product service initialized")
	}

	//if conf.Telegram.Enabled {
	//	tg, e := telegram.New(conf.Telegram.ApiKey, lg)
	//	if e != nil {
	//		lg.Error("telegram api", sl.Err(e))
	//	}
	//	//if mongo != nil {
	//	//	tg.SetDatabase(mongo)
	//	//}
	//	tg.Start()
	//	lg.Info("telegram api initialized")
	//	handler.SetMessageService(tg)
	//}

	// *** blocking start with http server ***
	err = api.New(conf, lg, handler)
	if err != nil {
		lg.Error("server start", sl.Err(err))
		return
	}
	lg.Error("service stopped")
}
