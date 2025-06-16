package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
)

type Repository interface {
	CheckApiKey(key string) (string, error)
}

type ProductService interface {
	ProductSearch(article string) (interface{}, error)
	GetProductInfo(articles []string) ([]entity.Product, error)
}

type MessageService interface {
	SendEventMessage(msg *entity.EventMessage) error
}

type Core struct {
	repo    Repository
	ms      MessageService
	ps      ProductService
	authKey string
	keys    map[string]string
	log     *slog.Logger
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

func (c *Core) SendMail(message *entity.MailMessage) (interface{}, error) {
	return nil, nil
}

func (c *Core) SendEvent(message *entity.EventMessage) (interface{}, error) {
	if c.ms == nil {
		return nil, fmt.Errorf("not set MessageService")
	}
	return nil, c.ms.SendEventMessage(message)
}
