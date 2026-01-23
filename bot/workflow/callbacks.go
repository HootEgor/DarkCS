package workflow

import (
	"strconv"
	"strings"
)

// Callback action constants
const (
	CallbackPrefix = "wf:"
	ActionYes      = "yes"
	ActionNo       = "no"
	ActionConfirm  = "confirm"
	ActionCancel   = "cancel"
	ActionPage     = "page"
	ActionSelect   = "select"
	ActionMenu     = "menu"
	ActionNoop     = "noop"
)

// CallbackData represents parsed callback data.
type CallbackData struct {
	Action string
	Value  string
}

// ParseCallback parses a callback data string.
// Format: "wf:action:value" or "wf:action"
func ParseCallback(data string) *CallbackData {
	if !strings.HasPrefix(data, CallbackPrefix) {
		return nil
	}

	data = strings.TrimPrefix(data, CallbackPrefix)
	parts := strings.SplitN(data, ":", 2)

	cb := &CallbackData{
		Action: parts[0],
	}

	if len(parts) > 1 {
		cb.Value = parts[1]
	}

	return cb
}

// IsWorkflowCallback checks if the callback data is a workflow callback.
func IsWorkflowCallback(data string) bool {
	return strings.HasPrefix(data, CallbackPrefix)
}

// BuildCallback creates a callback data string.
func BuildCallback(action string, value ...string) string {
	if len(value) > 0 && value[0] != "" {
		return CallbackPrefix + action + ":" + value[0]
	}
	return CallbackPrefix + action
}

// IsYes checks if the callback is a "yes" action.
func (c *CallbackData) IsYes() bool {
	return c.Action == ActionYes
}

// IsNo checks if the callback is a "no" action.
func (c *CallbackData) IsNo() bool {
	return c.Action == ActionNo
}

// IsConfirm checks if the callback is a "confirm" action.
func (c *CallbackData) IsConfirm() bool {
	return c.Action == ActionConfirm
}

// IsCancel checks if the callback is a "cancel" action.
func (c *CallbackData) IsCancel() bool {
	return c.Action == ActionCancel
}

// IsPage checks if the callback is a "page" action.
func (c *CallbackData) IsPage() bool {
	return c.Action == ActionPage
}

// IsSelect checks if the callback is a "select" action.
func (c *CallbackData) IsSelect() bool {
	return c.Action == ActionSelect
}

// IsMenu checks if the callback is a "menu" action.
func (c *CallbackData) IsMenu() bool {
	return c.Action == ActionMenu
}

// IsNoop checks if the callback is a "noop" action.
func (c *CallbackData) IsNoop() bool {
	return c.Action == ActionNoop
}

// PageNumber returns the page number for page callbacks.
func (c *CallbackData) PageNumber() int {
	if c.Action != ActionPage {
		return 0
	}
	n, err := strconv.Atoi(c.Value)
	if err != nil {
		return 0
	}
	return n
}

// SelectedID returns the selected item ID for select callbacks.
func (c *CallbackData) SelectedID() string {
	if c.Action != ActionSelect {
		return ""
	}
	return c.Value
}

// MenuID returns the menu item ID for menu callbacks.
func (c *CallbackData) MenuID() string {
	if c.Action != ActionMenu {
		return ""
	}
	return c.Value
}
