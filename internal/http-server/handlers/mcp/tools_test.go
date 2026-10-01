package mcp

import "testing"

func TestToolAllowed(t *testing.T) {
	tests := []struct {
		assistant, tool string
		want            bool
	}{
		{"Consultant", "get_products_info", true},
		{"Consultant", "create_order", false}, // shop tools are Order Manager only
		{"Order Manager", "create_order", true},
		{"Order Manager", "get_products_info", true},
		{"Order Manager", "update_user_phone", false}, // handler exists but is not advertised
		{"Order Manager", "no_such_tool", false},
	}
	for _, tt := range tests {
		if got := ToolAllowed(tt.assistant, tt.tool); got != tt.want {
			t.Errorf("ToolAllowed(%q, %q) = %v, want %v", tt.assistant, tt.tool, got, tt.want)
		}
	}
}
