package main

import (
	"DarkCS/ai/gpt"
	"DarkCS/bot"
	"DarkCS/impl/core"
	"DarkCS/internal/config"
	"DarkCS/internal/database"
	"DarkCS/internal/http-server/api"
	"DarkCS/internal/lib/logger"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/service/auth"
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

	// Initialize Telegram bot if enabled
	var tgBot *bot.TgBot
	if conf.Telegram.Enabled {
		var err error
		tgBot, err = bot.NewTgBot(conf.Telegram.BotName, conf.Telegram.ApiKey, conf.Telegram.AdminId, lg)
		if err != nil {
			lg.Error("failed to initialize telegram bot", slog.String("error", err.Error()))
		} else {
			// Set up Telegram handler for the logger
			lg = logger.SetupTelegramHandler(lg, tgBot, slog.LevelDebug)
			lg.With(
				slog.String("bot_name", conf.Telegram.BotName),
			).Info("telegram bot initialized")

			// Start the bot in a goroutine
			go func() {
				if err := tgBot.Start(); err != nil {
					lg.Error("telegram bot error", slog.String("error", err.Error()))
				}
			}()
		}
	}

	lg.Info("starting darkcs", slog.String("config", *configPath), slog.String("env", conf.Env))
	lg.Debug("debug messages enabled")

	handler := core.New(lg)
	handler.SetAuthKey(conf.Listen.ApiKey)

	authService := auth.NewAuthService(lg)

	db, err := repository.NewMongoClient(conf, lg)
	if err != nil {
		lg.With(
			sl.Err(err),
		).Error("mongo client")
	}
	if db != nil {
		authService.SetRepository(db)
		handler.SetRepository(db)
		handler.SetAuthService(authService)
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

	overseer := gpt.NewOverseer(conf, lg)
	if overseer != nil {
		overseer.SetProductService(ps)
		overseer.SetAuthService(authService)
		handler.SetAssistant(overseer)
		lg.With(
			sl.Secret("openai_key", conf.OpenAI.ApiKey),
			sl.Secret("overseer_id", conf.OpenAI.OverseerID),
		).Info("overseer initialized")
	}

	handler.Init()

	// *** blocking start with http server ***
	err = api.New(conf, lg, handler)
	if err != nil {
		lg.Error("server start", sl.Err(err))
		return
	}
	lg.Error("service stopped")
}
