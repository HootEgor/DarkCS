package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

type Repository interface {
	CheckApiKey(key string) (string, error)
	SaveMessage(message entity.Message) error
	GenerateApiKey(username string) (string, error)

	UpsertAssistant(assistant *entity.Assistant) (*entity.Assistant, error)
	GetAssistant(name string) (*entity.Assistant, error)
	GetAllAssistants() ([]entity.Assistant, error)

	GetAllQrStat() ([]entity.QrStat, error)

	FollowQr(smartSenderId string) error
	RegisterQr(smartSenderId string) error

	UpsertSchool(ctx context.Context, school *entity.School) error
	GetAllSchools(ctx context.Context) ([]entity.School, error)
	GetAllActiveSchools(ctx context.Context) ([]entity.School, error)
	GetInactiveSchools(ctx context.Context) ([]entity.School, error)
	SetSchoolActive(ctx context.Context, name string, active bool) error
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
	HandleCommand(user *entity.User, name string, args json.RawMessage) (interface{}, error)

	GetAudioText(fileURL string) (string, error)

	AttachNewFile() error
}

type AuthService interface {
	RegisterUser(name, email, phone string, telegramId int64) (*entity.User, error)
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	GetUserByUUID(uuid string) (*entity.User, error)
	GetUserByInstagramId(instagramId string) (*entity.User, error)
	UserExists(email, phone string, telegramId int64) (*entity.User, error)
	BlockUser(email, phone string, telegramId int64, block bool, role string) error
	UpdateUser(user *entity.User) error

	ActivatePromoCode(phone, code string) error

	GetActivePromoCodes() ([]entity.PromoCode, error)
	GeneratePromoCodes(number int) error
	GenerateRandomNumCode(length int) string
	SetSmartSenderId(email, phone string, telegramId int64, smartSenderId string) error

	UpdateConversation(user entity.User, conversation entity.DialogMessage) error
	ClearConversation(user *entity.User) error

	IsUserManager(email, phone string, telegramId int64) bool
}

type SmartService interface {
	EditLatestInputMessage(userId, text string) error
	SendMessage(userId, text string) error
}

type ZohoService interface {
	// CreateOrder creates a new order in the Zoho CRM system
	CreateOrder(order *entity.Order) error

	// GetOrders retrieves a list of orders for a specific user
	GetOrders(userInfo entity.UserInfo) ([]entity.OrderStatus, error)

	// GetOrdersDetailed retrieves detailed order information for a user
	GetOrdersDetailed(userInfo entity.UserInfo) ([]entity.OrderDetail, error)

	// GetOrdersDetailedByZohoId retrieves detailed order information using a stored Zoho contact ID
	GetOrdersDetailedByZohoId(zohoId string) ([]entity.OrderDetail, error)

	GetOrderProducts(orderId string) (string, error)

	CreateContact(user *entity.User) (string, error)

	// CreateRating creates a service rating in Zoho CRM
	CreateRating(rating entity.ServiceRating) error
}

type Core struct {
	repo         Repository
	ms           MessageService
	ps           ProductService
	ass          Assistant
	authService  AuthService
	smartService SmartService
	zoho         ZohoService
	authKey      string
	keys         map[string]string
	log          *slog.Logger
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

func (c *Core) SetSmartService(smart SmartService) {
	c.smartService = smart
}

func (c *Core) SetZohoService(zoho ZohoService) {
	c.zoho = zoho
}

func (c *Core) Init() {
	go func() {
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), 21, 00, 0, 0, now.Location())

			if now.After(nextRun) {
				nextRun = nextRun.Add(24 * time.Hour)
			}
			c.log.With(
				slog.Time("nextRun", nextRun),
			).Info("next assistants product list update")

			time.Sleep(time.Until(nextRun))

			_ = c.AttachNewFile()
		}
	}()
}

func (c *Core) SendMail(message *entity.MailMessage) (interface{}, error) {
	return nil, nil
}
