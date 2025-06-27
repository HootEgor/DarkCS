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
}

type ProductService interface {
	ProductSearch(article string) (interface{}, error)
	GetProductInfo(articles []string) ([]entity.ProductInfo, error)
}

type MessageService interface {
	SendEventMessage(msg *entity.EventMessage) error
}

type Assistant interface {
	ComposeResponse(user *entity.User, systemMsg, userMsg string) (entity.AiAnswer, error)

	AttachNewFile() error
}

type AuthService interface {
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	BlockUser(email, phone string, telegramId int64, block bool) error
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

func (c *Core) Init() {
	go func() {
		_ = c.ass.AttachNewFile()
		//for {
		//	now := time.Now()
		//	nextRun := time.Date(now.Year(), now.Month(), now.Day(), 19, 10, 0, 0, now.Location())
		//
		//	// Якщо вже після 20:30, переходимо на наступний день
		//	if now.After(nextRun) {
		//		nextRun = nextRun.Add(24 * time.Hour)
		//	}
		//	c.log.With(
		//		slog.Time("nextRun", nextRun),
		//	).Info("next assistants product list update")
		//
		//	// Очікуємо до наступного запуску
		//	time.Sleep(time.Until(nextRun))
		//
		//	err := c.ass.AttachNewFile()
		//	if err != nil {
		//		c.log.With(
		//			sl.Err(err),
		//		).Error("update assistants product list")
		//	} else {
		//		c.log.With(
		//			slog.String("info", "success"),
		//		).Info("update assistants product list")
		//	}
		//}
	}()
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
	return c.authService.BlockUser(email, phone, telegramId, block)
}

func (c *Core) GetUser(email, phone string, telegramId int64) (*entity.User, error) {
	return c.authService.GetUser(email, phone, telegramId)
}
