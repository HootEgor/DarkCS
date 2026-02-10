package main

import (
	"flag"
	"log/slog"

	"DarkCS/ai/gpt"
	"DarkCS/bot"
	"DarkCS/bot/chat"
	igmessenger "DarkCS/bot/chat/instagram"
	chatmainmenu "DarkCS/bot/chat/mainmenu"
	chatonboarding "DarkCS/bot/chat/onboarding"
	tgmessenger "DarkCS/bot/chat/telegram"
	wamessenger "DarkCS/bot/chat/whatsapp"
	"DarkCS/bot/insta"
	"DarkCS/bot/whatsapp"
	"DarkCS/impl/core"
	"DarkCS/internal/config"
	repository "DarkCS/internal/database"
	"DarkCS/internal/http-server/api"
	"DarkCS/internal/lib/logger"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/service/auth"
	"DarkCS/internal/service/product"
	"DarkCS/internal/service/smart-sender"
	services "DarkCS/internal/service/zoho"
	"DarkCS/internal/ws"
)

func main() {

	configPath := flag.String("conf", "config.yml", "path to config file")
	logPath := flag.String("log", "/var/log/", "path to log file directory")
	flag.Parse()

	conf := config.MustLoad(*configPath)
	lg := logger.SetupLogger(conf.Env, *logPath)

	// Initialize Telegram bot if enabled (start later after workflow engine is configured)
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
		}

		// Start admin telegram bot
		if tgBot != nil {
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

	// Variable to hold userBot for later start
	var userBot *bot.UserBot

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

		// Initialize user bot if enabled (will be wired with ChatEngine later)
		if conf.UserBot.Enabled {
			var err error
			userBot, err = bot.NewUserBot(conf.UserBot.BotName, conf.UserBot.ApiKey, lg)
			if err != nil {
				lg.Error("failed to initialize user bot", slog.String("error", err.Error()))
			}
		}
	}

	ps := product.NewProductService(conf, lg)
	if ps != nil {
		handler.SetProductService(ps)
		lg.With(
			slog.String("login", conf.ProdService.Login),
			slog.String("url", conf.ProdService.BaseURL),
		).Info("product service initialized")
	}

	zohoService := services.NewZohoService(conf, lg)
	if zohoService != nil {
		lg.Debug("zoho service initialized")
	}

	mcpApiKey, err := handler.GenerateApiKey("openai")
	if err != nil {
		lg.With(
			sl.Err(err),
		).Error("generate openai api key")
	}

	overseer := gpt.NewOverseer(conf, lg, mcpApiKey)
	if overseer != nil {
		overseer.SetRepository(db)
		overseer.SetZohoService(zohoService)
		overseer.SetProductService(ps)
		overseer.SetAuthService(authService)
		handler.SetAssistant(overseer)
		lg.With(
			sl.Secret("openai_key", conf.OpenAI.ApiKey),
			sl.Secret("overseer_id", conf.OpenAI.OverseerID),
		).Info("overseer initialized")
	}

	smartService := smart_sender.NewSmartSenderService(conf, lg)
	handler.SetSmartService(smartService)
	handler.SetZohoService(zohoService)

	// Create WebSocket hub for CRM
	wsHub := ws.NewHub()
	go wsHub.Run()
	handler.SetWsHub(wsHub)

	handler.Init()

	// Initialize unified ChatEngine shared by all platforms (Telegram, Instagram, WhatsApp)
	var chatEngine *chat.ChatEngine
	if db != nil {
		chatStateStorage := chat.NewMongoChatStateStorage(db)
		chatEngine = chat.NewChatEngine(chatStateStorage, lg)

		// Register chat workflows
		chatOnboarding := chatonboarding.NewOnboardingWorkflow(authService, zohoService, lg)
		chatEngine.RegisterWorkflow(chatOnboarding)

		chatMainMenu := chatmainmenu.NewMainMenuWorkflow(authService, zohoService, handler, lg)
		chatEngine.RegisterWorkflow(chatMainMenu)

		// Wire message listener for CRM
		chatEngine.SetMessageListener(handler)

		lg.Info("chat engine initialized")
	}

	// Wire ChatEngine into user bot and start
	if userBot != nil && chatEngine != nil {
		userBot.SetChatEngine(chatEngine)
		handler.SetPlatformMessenger("telegram", tgmessenger.NewMessenger(userBot.GetAPI()))
		go func() {
			if err := userBot.Start(); err != nil {
				lg.Error("user bot error", slog.String("error", err.Error()))
			}
		}()
	}

	// Initialize Instagram bot if enabled
	var instaBot *insta.InstaBot
	if conf.Instagram.Enabled {
		instaBot = insta.NewInstaBot(
			conf.Instagram.AccessToken,
			conf.Instagram.VerifyToken,
			conf.Instagram.AppSecret,
			lg,
		)
		if chatEngine != nil {
			instaBot.SetChatEngine(chatEngine)
		}
		handler.SetPlatformMessenger("instagram", igmessenger.NewMessenger(instaBot))
		lg.Info("instagram bot initialized")
	}

	// Initialize WhatsApp bot if enabled
	var whatsappBot *whatsapp.WhatsAppBot
	if conf.WhatsApp.Enabled {
		whatsappBot = whatsapp.NewWhatsAppBot(
			conf.WhatsApp.AccessToken,
			conf.WhatsApp.VerifyToken,
			conf.WhatsApp.AppSecret,
			conf.WhatsApp.PhoneNumberID,
			lg,
		)
		if chatEngine != nil {
			whatsappBot.SetChatEngine(chatEngine)
		}
		handler.SetPlatformMessenger("whatsapp", wamessenger.NewMessenger(whatsappBot))
		lg.Info("whatsapp bot initialized")
	}

	// *** blocking start with http server ***
	var apiOpts []api.Option
	if instaBot != nil {
		apiOpts = append(apiOpts, api.WithInstaBot(instaBot))
	}
	if whatsappBot != nil {
		apiOpts = append(apiOpts, api.WithWhatsAppBot(whatsappBot))
	}
	apiOpts = append(apiOpts, api.WithWsHub(wsHub, handler))
	err = api.New(conf, lg, handler, apiOpts...)
	if err != nil {
		lg.Error("server start", sl.Err(err))
		return
	}
	lg.Error("service stopped")
}
