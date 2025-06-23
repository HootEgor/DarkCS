package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
)

type Repository interface {
	CheckApiKey(key string) (string, error)
	SaveMessage(message entity.Message) error
	BlockUser(email, phone string, telegramId int64, block bool) error
}

type ProductService interface {
	ProductSearch(article string) (interface{}, error)
	GetProductInfo(articles []string) ([]entity.ProductInfo, error)
}

type MessageService interface {
	SendEventMessage(msg *entity.EventMessage) error
}

type Assistant interface {
	ComposeResponse(userId, systemMsg, userMsg string) (entity.AiAnswer, error)
}

type AuthService interface {
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
}

type Core struct {
	repo        Repository
	ms          MessageService
	ps          ProductService
	ass         Assistant
	authService AuthService
	authKey     string
	keys        map[string]string
	log         *slog.Logger
}

func New(log *slog.Logger) *Core {
	return &Core{
		log:  log.With(sl.Module("core")),
		keys: make(map[string]string),
	}
}

func (c *Core) SetRepository(repo Repository) {
	c.repo = repo
}

func (c *Core) SetAuthKey(key string) {
	c.authKey = key
}

func (c *Core) SetMessageService(ms MessageService) {
	c.ms = ms
}

func (c *Core) SetProductService(ps ProductService) {
	c.ps = ps
}

func (c *Core) SetAuthService(auth AuthService) {
	c.authService = auth
}

func (c *Core) SetAssistant(ass Assistant) {
	c.ass = ass
}

func (c *Core) SendMail(message *entity.MailMessage) (interface{}, error) {
	return nil, nil
}

func (c *Core) SendEvent(message *entity.EventMessage) (interface{}, error) {
	if c.ms == nil {
		return nil, fmt.Errorf("not set MessageService")
	}
	return nil, c.ms.SendEventMessage(message)
}

func (c *Core) BlockUser(email, phone string, telegramId int64, block bool) error {
	return c.repo.BlockUser(email, phone, telegramId, block)
}
