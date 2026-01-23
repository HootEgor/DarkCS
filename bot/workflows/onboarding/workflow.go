package onboarding

import (
	"DarkCS/bot/workflow"
	"context"
	"log/slog"

	"DarkCS/entity"
)

// Workflow ID
const (
	WorkflowID workflow.WorkflowID = "onboarding"
)

// Step IDs
const (
	StepHello           workflow.StepID = "hello"
	StepRequestPhone    workflow.StepID = "request_phone"
	StepValidatePhone   workflow.StepID = "validate_phone"
	StepCheckUser       workflow.StepID = "check_user"
	StepRequestName     workflow.StepID = "request_name"
	StepConfirmData     workflow.StepID = "confirm_data"
	StepProcessDeepCode workflow.StepID = "process_deep_code"
	StepSelectSchool    workflow.StepID = "select_school"
	StepMainMenu        workflow.StepID = "main_menu"
)

// State data keys
const (
	KeyPhone      = "phone"
	KeyName       = "name"
	KeyUserExists = "user_exists"
	KeyUserUUID   = "user_uuid"
	KeySchoolID   = "school_id"
	KeySchoolName = "school_name"
)

// AuthService defines the interface for user authentication operations.
type AuthService interface {
	UserExists(email, phone string, telegramId int64) (*entity.User, error)
	RegisterUser(name, email, phone string, telegramId int64) (*entity.User, error)
	UpdateUser(user *entity.User) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
}

// SchoolRepository defines the interface for school operations.
type SchoolRepository interface {
	GetSchoolByCode(ctx context.Context, code string) (*entity.School, error)
	GetSchoolByID(ctx context.Context, id string) (*entity.School, error)
	GetAllActiveSchools(ctx context.Context) ([]entity.School, error)
}

// OnboardingWorkflow implements the onboarding workflow.
type OnboardingWorkflow struct {
	steps       map[workflow.StepID]workflow.Step
	authService AuthService
	schoolRepo  SchoolRepository
	log         *slog.Logger
}

// NewOnboardingWorkflow creates a new onboarding workflow.
func NewOnboardingWorkflow(authService AuthService, schoolRepo SchoolRepository, log *slog.Logger) *OnboardingWorkflow {
	w := &OnboardingWorkflow{
		steps:       make(map[workflow.StepID]workflow.Step),
		authService: authService,
		schoolRepo:  schoolRepo,
		log:         log,
	}

	// Register all steps
	w.registerSteps()

	return w
}

// ID returns the workflow ID.
func (w *OnboardingWorkflow) ID() workflow.WorkflowID {
	return WorkflowID
}

// InitialStep returns the first step.
func (w *OnboardingWorkflow) InitialStep() workflow.StepID {
	return StepHello
}

// GetStep returns a step by ID.
func (w *OnboardingWorkflow) GetStep(id workflow.StepID) (workflow.Step, bool) {
	step, ok := w.steps[id]
	return step, ok
}

// Steps returns all steps.
func (w *OnboardingWorkflow) Steps() []workflow.Step {
	steps := make([]workflow.Step, 0, len(w.steps))
	for _, step := range w.steps {
		steps = append(steps, step)
	}
	return steps
}

// registerSteps registers all workflow steps.
func (w *OnboardingWorkflow) registerSteps() {
	w.steps[StepHello] = NewHelloStep()
	w.steps[StepRequestPhone] = NewRequestPhoneStep()
	w.steps[StepValidatePhone] = NewValidatePhoneStep()
	w.steps[StepCheckUser] = NewCheckUserStep(w.authService)
	w.steps[StepRequestName] = NewRequestNameStep()
	w.steps[StepConfirmData] = NewConfirmDataStep(w.authService)
	w.steps[StepProcessDeepCode] = NewProcessDeepCodeStep()
	w.steps[StepSelectSchool] = NewSelectSchoolStep(w.schoolRepo)
	w.steps[StepMainMenu] = NewMainMenuStep()
}
