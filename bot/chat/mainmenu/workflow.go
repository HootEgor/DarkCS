package mainmenu

import (
	"context"

	"DarkCS/bot/chat"
	"DarkCS/entity"
	"log/slog"
)

const (
	WorkflowID chat.WorkflowID = "mainmenu"
)

// Step IDs
const (
	StepSelectSchool    chat.StepID = "select_school"
	StepPreMainMenu     chat.StepID = "pre_main_menu"
	StepMainMenu        chat.StepID = "main_menu"
	StepMyOffice        chat.StepID = "my_office"
	StepCurrentOrder    chat.StepID = "current_order"
	StepCompletedOrders chat.StepID = "completed_orders"
	StepServiceRate     chat.StepID = "service_rate"
	StepAIConsultant    chat.StepID = "ai_consultant"
	StepMakeOrder       chat.StepID = "make_order"
	StepSchoolStat      chat.StepID = "school_stat"
)

// Menu button texts (same as Telegram)
const (
	BtnMyOffice        = "📦Особистий кабінет"
	BtnServiceRate     = "⭐Оцінка сервісу"
	BtnOrderStatus     = "🛒Статус замовлення"
	BtnAIConsultant    = "👋 AI-консультація"
	BtnMakeOrder       = "Зробити замовлення😎"
	BtnCurrentOrder    = "🛍️Поточні замовлення"
	BtnCompletedOrders = "✅Виконані замовлення"
	BtnBack            = "↩️Назад"
	BtnSchoolStat      = "📊Статистика шкіл"
)

// AuthService defines the interface for user operations.
type AuthService interface {
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	GetUserByInstagramId(instagramId string) (*entity.User, error)
}

// ZohoService defines the interface for Zoho CRM operations.
type ZohoService interface {
	GetOrdersDetailed(userInfo entity.UserInfo) ([]entity.OrderDetail, error)
	GetOrdersDetailedByZohoId(zohoId string) ([]entity.OrderDetail, error)
	GetOrderProducts(orderId string) (string, error)
	CreateRating(rating entity.ServiceRating) error
	CreateContact(user *entity.User) (string, error)
	UpdateContactSchool(zohoID, schoolName string) error
}

// AIService defines the interface for AI assistant operations.
type AIService interface {
	ProcessUserRequest(user *entity.User, message string) (*entity.AiAnswer, error)
}

// SchoolRepository defines the interface for school data access.
type SchoolRepository interface {
	GetAllActiveSchools(ctx context.Context) ([]entity.School, error)
}

// QrStatRepository defines data access for QR scan statistics.
type QrStatRepository interface {
	GetAllQrStat() ([]entity.QrStat, error)
	SaveSchoolStat(platform, userID, schoolName string) error
}

// MainMenuWorkflow implements the main menu for chat platforms.
type MainMenuWorkflow struct {
	steps map[chat.StepID]chat.Step
}

func NewMainMenuWorkflow(authService AuthService, zohoService ZohoService, aiService AIService, schoolRepo SchoolRepository, qrStatRepo QrStatRepository, log *slog.Logger) *MainMenuWorkflow {
	w := &MainMenuWorkflow{
		steps: make(map[chat.StepID]chat.Step),
	}

	w.steps[StepSelectSchool] = &SelectSchoolStep{schoolRepo: schoolRepo, authService: authService, zohoService: zohoService, qrStatRepo: qrStatRepo}
	w.steps[StepPreMainMenu] = &PreMainMenuStep{}
	w.steps[StepMainMenu] = &MainMenuStep{authService: authService}
	w.steps[StepMyOffice] = &MyOfficeStep{}
	w.steps[StepCurrentOrder] = &CurrentOrderStep{authService: authService, zohoService: zohoService}
	w.steps[StepCompletedOrders] = &CompletedOrdersStep{authService: authService, zohoService: zohoService}
	w.steps[StepServiceRate] = &ServiceRateStep{authService: authService, zohoService: zohoService}
	w.steps[StepAIConsultant] = &AIConsultantStep{authService: authService, aiService: aiService}
	w.steps[StepMakeOrder] = &MakeOrderStep{authService: authService, aiService: aiService}
	w.steps[StepSchoolStat] = &SchoolStatStep{qrStatRepo: qrStatRepo}

	return w
}

func (w *MainMenuWorkflow) ID() chat.WorkflowID      { return WorkflowID }
func (w *MainMenuWorkflow) InitialStep() chat.StepID { return StepSelectSchool }

func (w *MainMenuWorkflow) GetStep(id chat.StepID) (chat.Step, bool) {
	step, ok := w.steps[id]
	return step, ok
}
