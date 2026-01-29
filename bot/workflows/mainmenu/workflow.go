package mainmenu

import (
	"DarkCS/bot/workflow"
	"DarkCS/entity"
	"log/slog"
)

// Workflow ID
const (
	WorkflowID workflow.WorkflowID = "mainmenu"
)

// Step IDs
const (
	StepMainMenu        workflow.StepID = "main_menu"
	StepMyOffice        workflow.StepID = "my_office"
	StepCurrentOrder    workflow.StepID = "current_order"
	StepCompletedOrders workflow.StepID = "completed_orders"
	StepServiceRate     workflow.StepID = "service_rate"
	StepAIConsultant    workflow.StepID = "ai_consultant"
	StepMakeOrder       workflow.StepID = "make_order"
)

// State data keys
const (
	KeyAIMode       = "ai_mode"
	KeySelectedRate = "selected_rate"
)

// Menu button texts
const (
	BtnMyOffice        = "Мій кабінет"
	BtnServiceRate     = "Оцінка сервісу"
	BtnOrderStatus     = "Статус замовлення"
	BtnAIConsultant    = "AI консультант"
	BtnMakeOrder       = "Зробити замовлення"
	BtnCurrentOrder    = "Поточне замовлення"
	BtnCompletedOrders = "Виконані замовлення"
	BtnBack            = "Назад"
)

// AuthService defines the interface for user authentication operations.
type AuthService interface {
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
}

// ZohoService defines the interface for Zoho CRM operations.
type ZohoService interface {
	GetOrdersDetailed(userInfo entity.UserInfo) ([]entity.OrderDetail, error)
	GetOrdersDetailedByZohoId(zohoId string) ([]entity.OrderDetail, error)
	GetOrderProducts(orderId string) (string, error)
	CreateRating(rating entity.ServiceRating) error
	CreateContact(user *entity.User) (string, error)
}

// AIService defines the interface for AI assistant operations.
type AIService interface {
	ComposeResponse(user *entity.User, systemMsg, userMsg string) (entity.AiAnswer, error)
}

// MainMenuWorkflow implements the main menu workflow.
type MainMenuWorkflow struct {
	steps       map[workflow.StepID]workflow.Step
	authService AuthService
	zohoService ZohoService
	aiService   AIService
	log         *slog.Logger
}

// NewMainMenuWorkflow creates a new main menu workflow.
func NewMainMenuWorkflow(authService AuthService, zohoService ZohoService, aiService AIService, log *slog.Logger) *MainMenuWorkflow {
	w := &MainMenuWorkflow{
		steps:       make(map[workflow.StepID]workflow.Step),
		authService: authService,
		zohoService: zohoService,
		aiService:   aiService,
		log:         log,
	}

	// Register all steps
	w.registerSteps()

	return w
}

// ID returns the workflow ID.
func (w *MainMenuWorkflow) ID() workflow.WorkflowID {
	return WorkflowID
}

// InitialStep returns the first step.
func (w *MainMenuWorkflow) InitialStep() workflow.StepID {
	return StepMainMenu
}

// GetStep returns a step by ID.
func (w *MainMenuWorkflow) GetStep(id workflow.StepID) (workflow.Step, bool) {
	step, ok := w.steps[id]
	return step, ok
}

// Steps returns all steps.
func (w *MainMenuWorkflow) Steps() []workflow.Step {
	steps := make([]workflow.Step, 0, len(w.steps))
	for _, step := range w.steps {
		steps = append(steps, step)
	}
	return steps
}

// registerSteps registers all workflow steps.
func (w *MainMenuWorkflow) registerSteps() {
	w.steps[StepMainMenu] = NewMainMenuStep()
	w.steps[StepMyOffice] = NewMyOfficeStep()
	w.steps[StepCurrentOrder] = NewCurrentOrderStep(w.authService, w.zohoService)
	w.steps[StepCompletedOrders] = NewCompletedOrdersStep(w.authService, w.zohoService)
	w.steps[StepServiceRate] = NewServiceRateStep(w.authService, w.zohoService)
	w.steps[StepAIConsultant] = NewAIConsultantStep(w.authService, w.aiService)
	w.steps[StepMakeOrder] = NewMakeOrderStep(w.authService, w.aiService)
}
