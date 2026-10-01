package logger

import (
	"DarkCS/bot"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	envLocal    = "local"
	envDev      = "dev"
	envProd     = "prod"
	logFileName = "darkcs.log"
)

func SetupLogger(env, path string) *slog.Logger {
	var logger *slog.Logger
	var logFile *os.File
	var err error

	if env != envLocal {
		logPath := logFilePath(path)
		logFile, err = os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0640)
		if err != nil {
			log.Fatal("error opening log file: ", err)
		}
		log.Printf("env: %s; log file: %s", env, logPath)
	}

	switch env {
	case envLocal:
		logger = slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envDev:
		logger = slog.New(
			slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		logger = slog.New(
			slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	default:
		log.Fatal("invalid environment: ", env)
	}

	return logger
}

func logFilePath(path string) string {
	return filepath.Join(path, logFileName)
}

// SetupTelegramHandler wraps the logger so records at or above minLevel are also sent to
// the admin Telegram bot. The returned handler must be flushed on shutdown; it is nil
// when tgBot is nil.
func SetupTelegramHandler(logger *slog.Logger, tgBot *bot.TgBot, minLevel slog.Level) (*slog.Logger, *TelegramHandler) {
	if tgBot == nil {
		return logger, nil
	}
	tgHandler := NewTelegramHandler(logger.Handler(), tgBot, minLevel)
	return slog.New(tgHandler), tgHandler
}

// ParseLevel converts a config level name (debug, info, warn, error) to a slog.Level,
// falling back to def for empty or unknown names.
func ParseLevel(name string, def slog.Level) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.TrimSpace(name))); err != nil {
		return def
	}
	return l
}
