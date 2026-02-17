package core

import (
	"DarkCS/bot/chat"
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/ws"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Repository interface {
	CheckApiKey(key string) (string, error)
	SaveMessage(message entity.Message) error
	GenerateApiKey(username string) (string, error)

	SaveChatMessage(msg entity.ChatMessage) error
	GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error)
	GetActiveChats() ([]entity.ChatSummary, error)
	CountUnreadPerChat(receipts map[string]time.Time) (map[string]int, error)
	CleanupChatMessages() error
	EnsureChatMessageIndexes() error

	UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error)
	DownloadFile(fileID primitive.ObjectID) (string, entity.FileMetadata, io.ReadCloser, error)

	UpsertReadReceipt(username, platform, userID string, readAt time.Time) error
	GetReadReceipts(username string) ([]entity.ChatReadReceipt, error)
	EnsureReadReceiptIndexes() error

	SaveChatState(ctx context.Context, state *chat.ChatState) error

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
	GetSchoolByCode(ctx context.Context, code string) (*entity.School, error)
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
	GetUserBySmartSenderId(smartSenderId string) (*entity.User, error)
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

// ZohoFunctionsService buffers and sends chat messages to Zoho Functions.
type ZohoFunctionsService interface {
	BufferMessage(contactID string, item entity.ZohoMessageItem)
}

type Core struct {
	repo          Repository
	ms            MessageService
	ps            ProductService
	ass           Assistant
	authService   AuthService
	smartService  SmartService
	zoho          ZohoService
	zohoFn        ZohoFunctionsService
	authKey       string
	signingSecret string
	publicURL     string
	keys          map[string]string
	log           *slog.Logger
	wsHub         *ws.Hub
	messengers    map[string]chat.Messenger
}

func New(log *slog.Logger) *Core {
	return &Core{
		log:        log.With(sl.Module("core")),
		keys:       make(map[string]string),
		messengers: make(map[string]chat.Messenger),
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

func (c *Core) SetZohoFunctionsService(zf ZohoFunctionsService) {
	c.zohoFn = zf
}

func (c *Core) SetWsHub(hub *ws.Hub) {
	c.wsHub = hub
}

// SetSigningSecret sets the HMAC secret used to sign file download URLs.
func (c *Core) SetSigningSecret(secret string) {
	c.signingSecret = secret
}

// SetPublicURL sets the public base URL used for file download links sent to external platforms.
func (c *Core) SetPublicURL(url string) {
	c.publicURL = url
}

// FileSigningSecret returns the HMAC secret used to sign file download URLs.
func (c *Core) FileSigningSecret() string {
	return c.signingSecret
}

func (c *Core) SetPlatformMessenger(platform string, m chat.Messenger) {
	c.messengers[platform] = m
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

	// Chat message cleanup scheduler — runs daily at 03:00
	go func() {
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if now.After(nextRun) {
				nextRun = nextRun.Add(24 * time.Hour)
			}
			c.log.With(
				slog.Time("nextRun", nextRun),
			).Info("next chat message cleanup")

			time.Sleep(time.Until(nextRun))

			if err := c.repo.CleanupChatMessages(); err != nil {
				c.log.Error("chat message cleanup failed", slog.String("error", err.Error()))
			}
		}
	}()

	// Ensure chat message indexes
	if err := c.repo.EnsureChatMessageIndexes(); err != nil {
		c.log.Error("failed to ensure chat message indexes", slog.String("error", err.Error()))
	}

	// Ensure read receipt indexes
	if err := c.repo.EnsureReadReceiptIndexes(); err != nil {
		c.log.Error("failed to ensure read receipt indexes", slog.String("error", err.Error()))
	}
}

func (c *Core) SendMail(message *entity.MailMessage) (interface{}, error) {
	return nil, nil
}
