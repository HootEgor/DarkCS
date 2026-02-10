package chat

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ChatEngine is the platform-agnostic workflow orchestrator.
type ChatEngine struct {
	workflows       map[WorkflowID]Workflow
	storage         ChatStateStorage
	log             *slog.Logger
	messageListener MessageListener
}

// NewChatEngine creates a new chat engine.
func NewChatEngine(storage ChatStateStorage, log *slog.Logger) *ChatEngine {
	return &ChatEngine{
		workflows: make(map[WorkflowID]Workflow),
		storage:   storage,
		log:       log,
	}
}

// SetMessageListener sets the listener for incoming messages.
func (e *ChatEngine) SetMessageListener(l MessageListener) {
	e.messageListener = l
}

// GetMessageListener returns the message listener (may be nil).
func (e *ChatEngine) GetMessageListener() MessageListener {
	return e.messageListener
}

// RegisterWorkflow adds a workflow to the engine.
func (e *ChatEngine) RegisterWorkflow(w Workflow) {
	e.workflows[w.ID()] = w
	e.log.Info("chat engine: registered workflow", slog.String("workflow_id", string(w.ID())))
}

// HandleMessage processes a text message from any platform.
func (e *ChatEngine) HandleMessage(ctx context.Context, m Messenger, platform, userID, chatID, text string) error {
	state, err := e.storage.Load(ctx, platform, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	// No active workflow — start onboarding
	if state == nil {
		return e.StartWorkflow(ctx, m, platform, userID, chatID, "onboarding")
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", state.WorkflowID)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		return fmt.Errorf("step not found: %s", state.CurrentStep)
	}

	input := UserInput{Text: text}
	result := step.HandleInput(ctx, m, state, input)
	return e.processResult(ctx, m, state, w, result)
}

// HandleCallback processes a callback/inline button press from any platform.
func (e *ChatEngine) HandleCallback(ctx context.Context, m Messenger, platform, userID, chatID, data string) error {
	state, err := e.storage.Load(ctx, platform, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if state == nil {
		return nil
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", state.WorkflowID)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		return fmt.Errorf("step not found: %s", state.CurrentStep)
	}

	input := UserInput{CallbackData: data}
	result := step.HandleInput(ctx, m, state, input)
	return e.processResult(ctx, m, state, w, result)
}

// HandleContact processes a contact share (phone number) from any platform.
func (e *ChatEngine) HandleContact(ctx context.Context, m Messenger, platform, userID, chatID, phone string) error {
	state, err := e.storage.Load(ctx, platform, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if state == nil {
		return nil
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", state.WorkflowID)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		return fmt.Errorf("step not found: %s", state.CurrentStep)
	}

	input := UserInput{Phone: phone}
	result := step.HandleInput(ctx, m, state, input)
	return e.processResult(ctx, m, state, w, result)
}

// StartWorkflow begins a new workflow for a user.
func (e *ChatEngine) StartWorkflow(ctx context.Context, m Messenger, platform, userID, chatID string, workflowID WorkflowID) error {
	w, ok := e.workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	state := NewChatState(platform, userID, chatID, workflowID, w.InitialStep())

	if err := e.storage.Save(ctx, state); err != nil {
		return fmt.Errorf("saving initial state: %w", err)
	}

	step, ok := w.GetStep(w.InitialStep())
	if !ok {
		return fmt.Errorf("initial step not found: %s", w.InitialStep())
	}

	e.log.Info("chat engine: starting workflow",
		slog.String("platform", platform),
		slog.String("user_id", userID),
		slog.String("workflow_id", string(workflowID)),
	)

	result := step.Enter(ctx, m, state)
	return e.processResult(ctx, m, state, w, result)
}

// processResult handles the result of a step handler — transitions, chaining, state saves.
func (e *ChatEngine) processResult(ctx context.Context, m Messenger, state *ChatState, w Workflow, result StepResult) error {
	if result.Error != nil {
		e.log.Error("chat engine: step error",
			slog.String("platform", state.Platform),
			slog.String("user_id", state.UserID),
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
		e.log.Info("chat engine: workflow completed",
			slog.String("platform", state.Platform),
			slog.String("user_id", state.UserID),
			slog.String("workflow_id", string(state.WorkflowID)),
		)

		// Check if there's a next workflow to chain to
		nextWorkflowID := state.GetString("next_workflow")
		if nextWorkflowID != "" {
			if err := e.storage.Delete(ctx, state.Platform, state.UserID); err != nil {
				return err
			}
			return e.StartWorkflow(ctx, m, state.Platform, state.UserID, state.ChatID, WorkflowID(nextWorkflowID))
		}

		return e.storage.Delete(ctx, state.Platform, state.UserID)
	}

	// Transition to next step if specified, looping through auto-transitions
	const maxTransitions = 20
	for i := 0; result.NextStep != "" && result.NextStep != state.CurrentStep && i < maxTransitions; i++ {
		state.CurrentStep = result.NextStep

		if err := e.storage.Save(ctx, state); err != nil {
			return fmt.Errorf("saving state after transition: %w", err)
		}

		step, ok := w.GetStep(result.NextStep)
		if !ok {
			return fmt.Errorf("next step not found: %s", result.NextStep)
		}

		e.log.Debug("chat engine: transitioning",
			slog.String("platform", state.Platform),
			slog.String("user_id", state.UserID),
			slog.String("step_id", string(result.NextStep)),
		)

		result = step.Enter(ctx, m, state)
		if result.Error != nil {
			return result.Error
		}

		if result.UpdateState != nil {
			state.MergeData(result.UpdateState)
		}
		state.UpdatedAt = time.Now()

		if result.Complete {
			e.log.Info("chat engine: workflow completed",
				slog.String("platform", state.Platform),
				slog.String("user_id", state.UserID),
				slog.String("workflow_id", string(state.WorkflowID)),
			)

			nextWorkflowID := state.GetString("next_workflow")
			if nextWorkflowID != "" {
				if err := e.storage.Delete(ctx, state.Platform, state.UserID); err != nil {
					return err
				}
				return e.StartWorkflow(ctx, m, state.Platform, state.UserID, state.ChatID, WorkflowID(nextWorkflowID))
			}

			return e.storage.Delete(ctx, state.Platform, state.UserID)
		}
	}

	return e.storage.Save(ctx, state)
}
