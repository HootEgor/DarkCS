package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log/slog"
	"path/filepath"
	"time"

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
	"DarkCS/internal/gdrive"
	"DarkCS/internal/http-server/api"
	"DarkCS/internal/lib/logger"
	"DarkCS/internal/lib/safego"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/service/auth"
	"DarkCS/internal/service/product"
	"DarkCS/internal/service/smart-sender"
	services "DarkCS/internal/service/zoho"
	zoho_functions "DarkCS/internal/service/zoho-functions"
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
				// tgBot.Start panics if polling cannot start; keep the rest of the service up.
				defer safego.Recover(lg, "admin telegram bot")
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
	handler.SetSigningSecret(fileSigningSecret(conf, lg))

	authService := auth.NewAuthService(lg)

	db, err := repository.NewMongoClient(conf, lg)
	if err != nil {
		// Almost every feature needs the database; exit so systemd restarts us rather
		// than serving requests that would all fail.
		lg.With(
			sl.Err(err),
		).Error("mongo client")
		return
	}
	if db != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = db.Close(ctx)
		}()
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

	mcpApiKey, err := handler.EnsureMcpKey("openai")
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
		).Info("overseer initialized")
	}

	smartService := smart_sender.NewSmartSenderService(conf, lg)
	handler.SetSmartService(smartService)
	handler.SetZohoService(zohoService)

	if conf.ZohoFunctions.MsgUrl != "" && conf.ZohoFunctions.ApiKey != "" {
		zohoFnService := zoho_functions.NewZohoFunctionsService(
			conf.ZohoFunctions.MsgUrl,
			conf.ZohoFunctions.ApiKey,
			lg,
		)
		handler.SetZohoFunctionsService(zohoFnService)
		lg.Info("zoho functions service initialized")
	}

	// Create WebSocket hub for CRM
	ws.SetAllowedOrigins(conf.Listen.AllowedOrigins)
	wsHub := ws.NewHub(lg)
	wsHub.SetHandler(handler)
	go wsHub.Run()
	handler.SetWsHub(wsHub)

	handler.Init()

	// Resolve the token file path. If not set in config, default to
	// gdrive_token.json in the same directory as the credentials file so the
	// token persists across restarts without requiring any config change.
	driveTokenFile := conf.GoogleDrive.TokenFile
	if driveTokenFile == "" && conf.GoogleDrive.CredentialsFile != "" {
		driveTokenFile = filepath.Join(filepath.Dir(conf.GoogleDrive.CredentialsFile), "gdrive_token.json")
	}

	// Initialize Google Drive service for training videos (optional).
	var driveService gdrive.DriveService
	if conf.GoogleDrive.Enabled && conf.GoogleDrive.CredentialsFile != "" {
		ttl := time.Duration(conf.GoogleDrive.CacheTTLMinutes) * time.Minute
		var driveErr error
		driveService, driveErr = gdrive.NewDriveService(conf.GoogleDrive.CredentialsFile, driveTokenFile, conf.GoogleDrive.FolderID, ttl, lg)
		if driveErr != nil {
			lg.Error("google drive init failed — training videos unavailable", sl.Err(driveErr))
		} else {
			lg.Info("google drive initialized", slog.String("folder_id", conf.GoogleDrive.FolderID))
		}
	} else {
		lg.Warn("google drive disabled or credentials not set — training videos unavailable",
			slog.Bool("enabled", conf.GoogleDrive.Enabled),
			slog.String("credentials_file", conf.GoogleDrive.CredentialsFile),
		)
	}

	// Wire Drive auth flow into the admin bot so /gdrive_auth can be used to
	// obtain and save the token without shell access to the server.
	if tgBot != nil && conf.GoogleDrive.CredentialsFile != "" {
		tgBot.SetDriveAuth(bot.DriveAuthConfig{
			CredFile:  conf.GoogleDrive.CredentialsFile,
			TokenFile: driveTokenFile,
		})
	}

	// Initialize unified ChatEngine shared by all platforms (Telegram, Instagram, WhatsApp)
	var chatEngine *chat.ChatEngine
	if db != nil {
		chatStateStorage := chat.NewMongoChatStateStorage(db)
		chatEngine = chat.NewChatEngine(chatStateStorage, lg)

		// Register chat workflows
		chatOnboarding := chatonboarding.NewOnboardingWorkflow(authService, zohoService, handler, lg)
		chatEngine.RegisterWorkflow(chatOnboarding)

		chatMainMenu := chatmainmenu.NewMainMenuWorkflow(authService, zohoService, handler, db, db, driveService, lg)
		chatEngine.RegisterWorkflow(chatMainMenu)

		// Wire message listener for CRM
		chatEngine.SetMessageListener(handler)

		lg.Info("chat engine initialized")
	}

	// Wire ChatEngine into user bot and start
	if userBot != nil && chatEngine != nil {
		userBot.SetChatEngine(chatEngine)
		userBot.SetAuthService(authService)
		handler.SetPlatformMessenger("telegram", tgmessenger.NewMessenger(userBot.GetAPI()))
		go func() {
			defer safego.Recover(lg, "user telegram bot")
			if err := userBot.Start(); err != nil {
				lg.Error("user bot error", slog.String("error", err.Error()))
			}
		}()
	}

	// Initialize Instagram bot if enabled
	var instaBot *insta.InstaBot
	if conf.Instagram.Enabled {
		// Prefer the token persisted by the last refresh: the config token is the one
		// originally issued and expires 60 days after that, while refreshed tokens don't.
		igToken := conf.Instagram.AccessToken
		storedToken := ""
		if db != nil {
			t, err := db.GetInstagramToken()
			if err != nil {
				lg.Error("failed to load persisted instagram token", slog.String("error", err.Error()))
			}
			storedToken = t
		}
		if storedToken != "" {
			igToken = storedToken
		}

		instaBot = insta.NewInstaBot(
			igToken,
			conf.Instagram.VerifyToken,
			conf.Instagram.AppSecret,
			lg,
		)
		if chatEngine != nil {
			instaBot.SetChatEngine(chatEngine)
		}
		handler.SetPlatformMessenger("instagram", igmessenger.NewMessenger(instaBot))
		instaBot.SetFallbackToken(conf.Instagram.AccessToken)
		// Persist the token to MongoDB so DarkBot can import it via the same server.
		// Seed it from config only when nothing is stored, never over a refreshed token.
		if db != nil {
			instaBot.SetTokenPersister(db.SaveInstagramToken)
			if storedToken == "" && igToken != "" {
				if err := db.SaveInstagramToken(igToken); err != nil {
					lg.Error("failed to persist initial instagram token", slog.String("error", err.Error()))
				}
			}
		}
		instaBot.StartTokenRefresh(context.Background())
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

// fileSigningSecret picks the HMAC secret for CRM file links: the dedicated secret, else
// the API key (previous behaviour), else a random per-process secret. An empty HMAC key
// would let anyone forge download links. Links live 15 minutes, so a changed secret only
// invalidates links issued just before a restart.
func fileSigningSecret(conf *config.Config, lg *slog.Logger) string {
	if conf.Listen.FileSigningSecret != "" {
		return conf.Listen.FileSigningSecret
	}
	if conf.Listen.ApiKey != "" {
		lg.Warn("listen.file_signing_secret not set; signing file links with the API key")
		return conf.Listen.ApiKey
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	lg.Warn("no file signing secret configured; using a random one (links break on restart)")
	return hex.EncodeToString(b)
}
