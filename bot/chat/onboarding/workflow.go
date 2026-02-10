package onboarding

import (
	"DarkCS/bot/chat"
	"DarkCS/entity"
	"log/slog"
)

const (
	WorkflowID chat.WorkflowID = "onboarding"
)

// Step IDs
const (
	StepHello        chat.StepID = "hello"
	StepChoosePhone  chat.StepID = "choose_phone"
	StepRequestPhone chat.StepID = "request_phone"
	StepCheckUser    chat.StepID = "check_user"
	StepRequestName  chat.StepID = "request_name"
	StepConfirmData  chat.StepID = "confirm_data"
	StepDone         chat.StepID = "done"
)

// State data keys
const (
	KeyPhone      = "phone"
	KeyName       = "name"
	KeyUserExists = "user_exists"
	KeyUserUUID   = "user_uuid"
)

// AuthService defines the interface for user operations.
type AuthService interface {
	UserExists(email, phone string, telegramId int64) (*entity.User, error)
	RegisterUser(name, email, phone string, telegramId int64) (*entity.User, error)
	UpdateUser(user *entity.User) error
	GetUserByInstagramId(instagramId string) (*entity.User, error)
}

// ZohoService defines the interface for Zoho CRM operations.
type ZohoService interface {
	CreateContact(user *entity.User) (string, error)
}

// OnboardingWorkflow implements the onboarding flow for chat platforms.
type OnboardingWorkflow struct {
	steps map[chat.StepID]chat.Step
}

func NewOnboardingWorkflow(authService AuthService, zohoService ZohoService, log *slog.Logger) *OnboardingWorkflow {
	w := &OnboardingWorkflow{
		steps: make(map[chat.StepID]chat.Step),
	}

	w.steps[StepHello] = &HelloStep{}
	w.steps[StepChoosePhone] = &ChoosePhoneStep{}
	w.steps[StepRequestPhone] = &RequestPhoneStep{}
	w.steps[StepCheckUser] = &CheckUserStep{authService: authService, zohoService: zohoService}
	w.steps[StepRequestName] = &RequestNameStep{}
	w.steps[StepConfirmData] = &ConfirmDataStep{authService: authService, zohoService: zohoService}
	w.steps[StepDone] = &DoneStep{}

	return w
}

func (w *OnboardingWorkflow) ID() chat.WorkflowID      { return WorkflowID }
func (w *OnboardingWorkflow) InitialStep() chat.StepID { return StepHello }

func (w *OnboardingWorkflow) GetStep(id chat.StepID) (chat.Step, bool) {
	step, ok := w.steps[id]
	return step, ok
}
