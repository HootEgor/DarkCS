package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// WorkflowEngine is the default implementation of the Engine interface.
type WorkflowEngine struct {
	workflows map[WorkflowID]Workflow
	storage   StateStorage
	log       *slog.Logger
}

// NewWorkflowEngine creates a new workflow engine.
func NewWorkflowEngine(storage StateStorage, log *slog.Logger) *WorkflowEngine {
	return &WorkflowEngine{
		workflows: make(map[WorkflowID]Workflow),
		storage:   storage,
		log:       log,
	}
}

// RegisterWorkflow adds a workflow to the engine.
func (e *WorkflowEngine) RegisterWorkflow(w Workflow) {
	e.workflows[w.ID()] = w
	e.log.Info("registered workflow", slog.String("workflow_id", string(w.ID())))
}

// StartWorkflow begins a new workflow for a user.
func (e *WorkflowEngine) StartWorkflow(ctx context.Context, b *tgbotapi.Bot, userID, chatID int64, workflowID WorkflowID, deepLink *DeepLinkData) error {
	w, ok := e.workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Create new state
	state := NewUserState(userID, chatID, workflowID, w.InitialStep())
	state.SetDeepLink(deepLink)

	// Save state
	if err := e.storage.Save(ctx, state); err != nil {
		return fmt.Errorf("saving initial state: %w", err)
	}

	// Enter the initial step
	step, ok := w.GetStep(w.InitialStep())
	if !ok {
		return fmt.Errorf("initial step not found: %s", w.InitialStep())
	}

	e.log.Info("starting workflow",
		slog.Int64("user_id", userID),
		slog.String("workflow_id", string(workflowID)),
		slog.String("step_id", string(w.InitialStep())),
	)

	return step.Enter(ctx, b, state)
}

// HandleMessage routes a message to the current workflow step.
func (e *WorkflowEngine) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context) error {
	userID := c.EffectiveUser.Id

	state, err := e.storage.Load(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if state == nil {
		return nil // No active workflow
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", state.WorkflowID)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		return fmt.Errorf("step not found: %s", state.CurrentStep)
	}

	result := step.HandleMessage(ctx, b, c, state)
	return e.processResult(ctx, b, state, w, result)
}

// HandleCallback routes a callback to the current workflow step.
func (e *WorkflowEngine) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, data string) error {
	userID := c.EffectiveUser.Id

	state, err := e.storage.Load(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if state == nil {
		return nil // No active workflow
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", state.WorkflowID)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		return fmt.Errorf("step not found: %s", state.CurrentStep)
	}

	result := step.HandleCallback(ctx, b, c, state, data)
	return e.processResult(ctx, b, state, w, result)
}

// HandleContact routes a contact to the current workflow step.
func (e *WorkflowEngine) HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context) error {
	userID := c.EffectiveUser.Id

	state, err := e.storage.Load(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if state == nil {
		return nil // No active workflow
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", state.WorkflowID)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		return fmt.Errorf("step not found: %s", state.CurrentStep)
	}

	result := step.HandleContact(ctx, b, c, state)
	return e.processResult(ctx, b, state, w, result)
}

// GetState retrieves the current state for a user.
func (e *WorkflowEngine) GetState(ctx context.Context, userID int64) (*UserState, error) {
	return e.storage.Load(ctx, userID)
}

// HasActiveWorkflow checks if a user has an active workflow.
func (e *WorkflowEngine) HasActiveWorkflow(ctx context.Context, userID int64) (bool, error) {
	return e.storage.Exists(ctx, userID)
}

// ClearState removes the workflow state for a user.
func (e *WorkflowEngine) ClearState(ctx context.Context, userID int64) error {
	return e.storage.Delete(ctx, userID)
}

// processResult handles the result of a step handler.
func (e *WorkflowEngine) processResult(ctx context.Context, b *tgbotapi.Bot, state *UserState, w Workflow, result StepResult) error {
	if result.Error != nil {
		e.log.Error("step error",
			slog.Int64("user_id", state.UserID),
			slog.String("step_id", string(state.CurrentStep)),
			slog.String("error", result.Error.Error()),
		)
		return result.Error
	}

	// Merge any state updates
	if result.UpdateState != nil {
		state.MergeData(result.UpdateState)
	}
	state.UpdatedAt = time.Now()

	// Check if workflow is complete
	if result.Complete {
		e.log.Info("workflow completed",
			slog.Int64("user_id", state.UserID),
			slog.String("workflow_id", string(state.WorkflowID)),
		)
		return e.storage.Delete(ctx, state.UserID)
	}

	// Transition to next step if specified
	if result.NextStep != "" && result.NextStep != state.CurrentStep {
		state.CurrentStep = result.NextStep

		// Save updated state
		if err := e.storage.Save(ctx, state); err != nil {
			return fmt.Errorf("saving state after transition: %w", err)
		}

		// Enter the new step
		step, ok := w.GetStep(result.NextStep)
		if !ok {
			return fmt.Errorf("next step not found: %s", result.NextStep)
		}

		e.log.Debug("transitioning to step",
			slog.Int64("user_id", state.UserID),
			slog.String("step_id", string(result.NextStep)),
		)

		return step.Enter(ctx, b, state)
	}

	// Just save state updates without transition
	return e.storage.Save(ctx, state)
}
