package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
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

	GetAudioText(fileURL string) (string, error)

	AttachNewFile() error
}

type AuthService interface {
	RegisterUser(name, email, phone string, telegramId int64) (*entity.User, error)
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	UserExists(email, phone string, telegramId int64) (*entity.User, error)
	BlockUser(email, phone string, telegramId int64, block bool) error
	UpdateUser(user *entity.User) error

	ActivatePromoCode(phone, code string) error

	GetActivePromoCodes() ([]entity.PromoCode, error)
	GeneratePromoCodes(number int) error
	GenerateRandomNumCode(length int) string
	SetSmartSenderId(email, phone string, telegramId int64, smartSenderId string) error
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

	GetOrderProducts(orderId string) (string, error)

	CreateContact(user *entity.User) (string, error)
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

func (c *Core) AttachNewFile() error {
	err := c.ass.AttachNewFile()
	if err != nil {
		c.log.With(
			sl.Err(err),
		).Error("update assistants product list")
	} else {
		c.log.With(
			slog.String("info", "success"),
		).Info("update assistants product list")
	}

	return err
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

func (c *Core) CreateUser(name, email, phone, smartSenderId string, telegramId int64) (string, string, error) {
	user, err := c.authService.RegisterUser(name, email, phone, telegramId)
	if err != nil {
		return "", "", fmt.Errorf("failed to create user: %w", err)
	}

	if user.SmartSenderId == "" && smartSenderId != "" {
		err = c.authService.SetSmartSenderId(email, phone, telegramId, smartSenderId)
		if err != nil {
			return "", "", err
		}
	}

	zohoId, err := c.zoho.CreateContact(user)
	if err != nil {
		return "", "", fmt.Errorf("failed to create zoho contact: %w", err)
	}

	return name, zohoId, nil
}

func (c *Core) GetOrderProducts(orderId, smartSenderId string) error {

	msg, err := c.zoho.GetOrderProducts(orderId)
	if err != nil {
		c.log.With(
			sl.Err(err),
		).Error("get order products")

		return c.smartService.SendMessage(smartSenderId, errorResponse)
	}

	return c.smartService.SendMessage(smartSenderId, msg)
}

func (c *Core) GeneratePromoCodes(number int) error {
	if c.authService == nil {
		return fmt.Errorf("authService is not set")
	}

	if number <= 0 {
		return fmt.Errorf("number of promo codes must be greater than zero")
	}

	return c.authService.GeneratePromoCodes(number)
}

func (c *Core) GetActivePromoCodes() ([]entity.PromoCode, error) {
	if c.authService == nil {
		return nil, fmt.Errorf("authService is not set")
	}

	codes, err := c.authService.GetActivePromoCodes()
	if err != nil {
		return nil, fmt.Errorf("failed to get active promo codes: %w", err)
	}

	if len(codes) == 0 {
		c.log.Info("No active promo codes found")
		return nil, nil // No active promo codes
	}

	return codes, nil
}

func (c *Core) UserActivatePromoCode(phone, code string) (bool, error) {
	if c.authService == nil {
		return false, fmt.Errorf("authService is not set")
	}

	if phone == "" {
		return false, fmt.Errorf("phone number is required")
	}

	err := c.authService.ActivatePromoCode(phone, code)
	if err != nil {
		return false, fmt.Errorf("failed to activate promo code: %w", err)
	}

	return true, nil
}

func (c *Core) UserHasPromoAccess(phone string) (bool, error) {
	user, err := c.authService.GetUser("", phone, 0)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return false, nil
	}

	return user.HasPromo(), nil
}

func (c *Core) ClosePromoForUser(phone string) error {
	user, err := c.authService.GetUser("", phone, 0)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.PromoExpire = time.Time{} // Reset promo expiration
	err = c.authService.UpdateUser(user)
	return err
}

func (c *Core) SendMessage(userId, text string) error {
	if c.smartService == nil {
		return fmt.Errorf("smartService is not set")
	}

	if userId == "" || text == "" {
		return fmt.Errorf("userId and text cannot be empty")
	}

	return c.smartService.SendMessage(userId, text)
}

func (c *Core) CheckUserPhone(phone string) (string, error) {
	if c.authService == nil {
		return "", fmt.Errorf("authService is not set")
	}

	phoneDigits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			phoneDigits += string(ch)
		}
	}
	phone = fmt.Sprintf("+%s", phoneDigits)

	user, err := c.authService.UserExists("", phone, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if user == nil {
		return "", nil
	}

	code := c.authService.GenerateRandomNumCode(4)

	codeMsg := fmt.Sprintf("Код авторизації:\n%s", code)

	if user.SmartSenderId == "" {
		return "", fmt.Errorf("user does not have a SmartSenderId set")

	}

	return code, c.smartService.SendMessage(user.SmartSenderId, codeMsg)
}

func (c *Core) GenerateApiKey(username string) (string, error) {
	if c.repo == nil {
		return "", fmt.Errorf("repository is not set")
	}

	apiKey, err := c.repo.GenerateApiKey(username)
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	c.keys[apiKey] = username
	return apiKey, nil
}

func (c *Core) UpdateAssistant(name, id string, active bool) error {
	if c.repo == nil {
		return fmt.Errorf("repository is not set")
	}

	assistant, _ := c.repo.GetAssistant(name)

	if assistant == nil {
		assistant = &entity.Assistant{
			Name: name,
		}
	}

	if id != "" {
		assistant.Id = id
	}

	assistant.Active = active
	_, err := c.repo.UpsertAssistant(assistant)
	if err != nil {
		return fmt.Errorf("failed to update assistant: %w", err)
	}

	return nil
}

func (c *Core) GetAllAssistants() ([]entity.Assistant, error) {
	if c.repo == nil {
		return nil, fmt.Errorf("repository is not set")
	}

	assistants, err := c.repo.GetAllAssistants()
	if err != nil {
		return nil, fmt.Errorf("failed to get all assistants: %w", err)
	}

	if len(assistants) == 0 {
		c.log.Info("No assistants found")
		return nil, nil // No assistants found
	}

	return assistants, nil
}
