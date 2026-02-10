package chat

import (
	"context"
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
	Enter(ctx context.Context, m Messenger, state *ChatState) StepResult

	// HandleInput processes user input (text, callback, or phone).
	HandleInput(ctx context.Context, m Messenger, state *ChatState, input UserInput) StepResult
}

// Workflow defines the interface for a complete workflow.
type Workflow interface {
	// ID returns the unique identifier for this workflow.
	ID() WorkflowID

	// InitialStep returns the first step of the workflow.
	InitialStep() StepID

	// GetStep returns a step by its ID.
	GetStep(id StepID) (Step, bool)
}

// ChatStateStorage handles persistence of chat states.
type ChatStateStorage interface {
	Save(ctx context.Context, state *ChatState) error
	Load(ctx context.Context, platform, userID string) (*ChatState, error)
	Delete(ctx context.Context, platform, userID string) error
}
