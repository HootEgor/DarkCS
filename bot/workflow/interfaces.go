package workflow

import (
	"context"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// StepID is a unique identifier for a step within a workflow.
type StepID string

// WorkflowID is a unique identifier for a workflow.
type WorkflowID string

// StepResult represents the outcome of handling an event in a step.
type StepResult struct {
	NextStep    StepID
	UpdateState map[string]any
	Complete    bool
	Error       error
}

// Step defines the interface for a single workflow step.
type Step interface {
	// ID returns the unique identifier for this step.
	ID() StepID

	// Enter is called when the user enters this step.
	// It should send any initial messages/keyboards to the user.
	// Return a StepResult with NextStep set to auto-transition without waiting for user input.
	Enter(ctx context.Context, b *tgbotapi.Bot, state *UserState) StepResult

	// HandleMessage processes a text message from the user.
	HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *UserState) StepResult

	// HandleCallback processes a callback query from inline keyboard buttons.
	HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *UserState, data string) StepResult

	// HandleContact processes a shared contact (phone number).
	HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *UserState) StepResult
}

// Workflow defines the interface for a complete workflow.
type Workflow interface {
	// ID returns the unique identifier for this workflow.
	ID() WorkflowID

	// InitialStep returns the first step of the workflow.
	InitialStep() StepID

	// GetStep returns a step by its ID.
	GetStep(id StepID) (Step, bool)

	// Steps returns all steps in this workflow.
	Steps() []Step
}

// Engine manages workflow execution and state persistence.
type Engine interface {
	// RegisterWorkflow adds a workflow to the engine.
	RegisterWorkflow(w Workflow)

	// StartWorkflow begins a new workflow for a user.
	StartWorkflow(ctx context.Context, b *tgbotapi.Bot, userID, chatID int64, workflowID WorkflowID, deepLink *DeepLinkData) error

	// HandleMessage routes a message to the current workflow step.
	HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context) error

	// HandleCallback routes a callback to the current workflow step.
	HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, data string) error

	// HandleContact routes a contact to the current workflow step.
	HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context) error

	// GetState retrieves the current state for a user.
	GetState(ctx context.Context, userID int64) (*UserState, error)

	// HasActiveWorkflow checks if a user has an active workflow.
	HasActiveWorkflow(ctx context.Context, userID int64) (bool, error)

	// ClearState removes the workflow state for a user.
	ClearState(ctx context.Context, userID int64) error
}

// StateStorage handles persistence of workflow states.
type StateStorage interface {
	// Save persists a user's workflow state.
	Save(ctx context.Context, state *UserState) error

	// Load retrieves a user's workflow state.
	Load(ctx context.Context, userID int64) (*UserState, error)

	// Delete removes a user's workflow state.
	Delete(ctx context.Context, userID int64) error

	// Exists checks if a user has a saved state.
	Exists(ctx context.Context, userID int64) (bool, error)
}
