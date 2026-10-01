package chat

import (
	"DarkCS/internal/lib/keymutex"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ChatEngine is the platform-agnostic workflow orchestrator.
type ChatEngine struct {
	workflows       map[WorkflowID]Workflow
	storage         ChatStateStorage
	log             *slog.Logger
	messageListener MessageListener

	// userLocks serializes load → step → save per {platform, user}. Without it two quick
	// messages (or a double-tapped button) run concurrently on copies of the same state,
	// the last save wins, and side effects such as registration or a Zoho rating repeat.
	userLocks *keymutex.KeyMutex
}

// lockUser blocks until no other update for this user is being processed.
func (e *ChatEngine) lockUser(platform, userID string) func() {
	return e.userLocks.Lock(platform + ":" + userID)
}

// NewChatEngine creates a new chat engine.
func NewChatEngine(storage ChatStateStorage, log *slog.Logger) *ChatEngine {
	return &ChatEngine{
		workflows: make(map[WorkflowID]Workflow),
		storage:   storage,
		log:       log,
		userLocks: keymutex.New(),
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

// restartKeywords reset the conversation on platforms without bot commands (Instagram,
// WhatsApp), where users otherwise have no way out of a broken or confusing state.
// Telegram has /start, handled by the bot itself.
var restartKeywords = map[string]bool{"start": true, "/start": true, "старт": true, "почати": true}

// HandleMessage processes a text message from any platform.
func (e *ChatEngine) HandleMessage(ctx context.Context, m Messenger, platform, userID, chatID, text string) error {
	defer e.lockUser(platform, userID)()
	m = newLoggingMessenger(m, e.messageListener, e.log, platform, userID)

	if platform != "telegram" && restartKeywords[strings.ToLower(strings.TrimSpace(text))] {
		if err := e.storage.Delete(ctx, platform, userID); err != nil {
			return fmt.Errorf("deleting state: %w", err)
		}
		return e.startWorkflowWithData(ctx, m, platform, userID, chatID, "onboarding", nil)
	}

	return e.dispatch(ctx, m, platform, userID, chatID, UserInput{Text: text}, true)
}

// HandleCallback processes a callback/inline button press from any platform.
// messageID is the ID of the message containing the inline keyboard (used for editing).
func (e *ChatEngine) HandleCallback(ctx context.Context, m Messenger, platform, userID, chatID, data, messageID string) error {
	defer e.lockUser(platform, userID)()
	m = newLoggingMessenger(m, e.messageListener, e.log, platform, userID)
	return e.dispatch(ctx, m, platform, userID, chatID, UserInput{CallbackData: data, MessageID: messageID}, false)
}

// HandleContact processes a contact share (phone number) from any platform.
// verified reports whether the platform confirmed the contact is the sender's own.
func (e *ChatEngine) HandleContact(ctx context.Context, m Messenger, platform, userID, chatID, phone string, verified bool) error {
	defer e.lockUser(platform, userID)()
	m = newLoggingMessenger(m, e.messageListener, e.log, platform, userID)
	return e.dispatch(ctx, m, platform, userID, chatID, UserInput{Phone: phone, PhoneVerified: verified}, false)
}

// dispatch routes input to the user's current step. Caller holds the user lock.
// startIfNone starts onboarding for users without state (text messages only).
//
// A state that points at a workflow or step that no longer exists (renamed or removed
// in a deploy) is reset instead of failing on every message, which would leave the
// user stuck with no way out on platforms without /start: an unknown step restarts its
// workflow, an unknown workflow restarts onboarding.
func (e *ChatEngine) dispatch(ctx context.Context, m Messenger, platform, userID, chatID string, input UserInput, startIfNone bool) error {
	state, err := e.storage.Load(ctx, platform, userID)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if state == nil {
		if startIfNone {
			return e.startWorkflowWithData(ctx, m, platform, userID, chatID, "onboarding", nil)
		}
		return nil
	}

	w, ok := e.workflows[state.WorkflowID]
	if !ok {
		e.log.Warn("chat engine: unknown workflow, restarting onboarding",
			slog.String("platform", platform), slog.String("user_id", userID),
			slog.String("workflow_id", string(state.WorkflowID)))
		if err = e.storage.Delete(ctx, platform, userID); err != nil {
			return fmt.Errorf("deleting state: %w", err)
		}
		return e.startWorkflowWithData(ctx, m, platform, userID, state.ChatID, "onboarding", nil)
	}

	step, ok := w.GetStep(state.CurrentStep)
	if !ok {
		e.log.Warn("chat engine: unknown step, restarting workflow",
			slog.String("platform", platform), slog.String("user_id", userID),
			slog.String("workflow_id", string(state.WorkflowID)), slog.String("step_id", string(state.CurrentStep)))
		if err = e.storage.Delete(ctx, platform, userID); err != nil {
			return fmt.Errorf("deleting state: %w", err)
		}
		return e.startWorkflowWithData(ctx, m, platform, userID, state.ChatID, w.ID(), nil)
	}

	result := step.HandleInput(ctx, m, state, input)
	return e.processResult(ctx, m, state, w, result)
}

// StartWorkflow begins a new workflow for a user.
func (e *ChatEngine) StartWorkflow(ctx context.Context, m Messenger, platform, userID, chatID string, workflowID WorkflowID) error {
	return e.StartWorkflowWithData(ctx, m, platform, userID, chatID, workflowID, nil)
}

// StartWorkflowWithData begins a new workflow for a user with initial state data.
func (e *ChatEngine) StartWorkflowWithData(ctx context.Context, m Messenger, platform, userID, chatID string, workflowID WorkflowID, initialData map[string]any) error {
	defer e.lockUser(platform, userID)()
	return e.startWorkflowWithData(ctx, m, platform, userID, chatID, workflowID, initialData)
}

// startWorkflowWithData is StartWorkflowWithData for callers already holding the user
// lock (message handling and workflow chaining); the lock is not reentrant.
func (e *ChatEngine) startWorkflowWithData(ctx context.Context, m Messenger, platform, userID, chatID string, workflowID WorkflowID, initialData map[string]any) error {
	m = newLoggingMessenger(m, e.messageListener, e.log, platform, userID)

	w, ok := e.workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	state := NewChatState(platform, userID, chatID, workflowID, w.InitialStep())
	if initialData != nil {
		state.MergeData(initialData)
	}

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
			return e.startWorkflowWithData(ctx, m, state.Platform, state.UserID, state.ChatID, WorkflowID(nextWorkflowID), deepLinkData(state))
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
				return e.startWorkflowWithData(ctx, m, state.Platform, state.UserID, state.ChatID, WorkflowID(nextWorkflowID), deepLinkData(state))
			}

			return e.storage.Delete(ctx, state.Platform, state.UserID)
		}
	}

	return e.storage.Save(ctx, state)
}

// ResetUsersAtSteps moves all users currently at any of the given steps to targetStep.
// Returns the count of affected states. Intended for admin use only.
func (e *ChatEngine) ResetUsersAtSteps(ctx context.Context, workflowID WorkflowID, steps []StepID, targetStep StepID) (int, error) {
	states, err := e.storage.FindBySteps(ctx, workflowID, steps)
	if err != nil {
		return 0, fmt.Errorf("finding states by steps: %w", err)
	}

	for _, state := range states {
		state.CurrentStep = targetStep
		if err := e.storage.Save(ctx, state); err != nil {
			return 0, fmt.Errorf("saving reset state for %s/%s: %w", state.Platform, state.UserID, err)
		}
	}

	e.log.Info("chat engine: reset users at steps",
		slog.String("workflow_id", string(workflowID)),
		slog.String("target_step", string(targetStep)),
		slog.Int("count", len(states)),
	)

	return len(states), nil
}

// deepLinkData extracts deep link keys from state to carry through workflow chaining.
func deepLinkData(state *ChatState) map[string]any {
	dlType := state.GetString("deep_link_type")
	dlID := state.GetString("deep_link_id")
	if dlType == "" && dlID == "" {
		return nil
	}
	data := make(map[string]any)
	if dlType != "" {
		data["deep_link_type"] = dlType
	}
	if dlID != "" {
		data["deep_link_id"] = dlID
	}
	return data
}
