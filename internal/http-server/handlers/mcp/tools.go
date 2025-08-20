package mcp

import (
	"encoding/json"
	"net/http"
)

func ToolsDescription(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tools := map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "get_products_info",
				"description": "Fetches information about products based on product codes",
				"parameters": map[string]interface{}{
					"type":     "object",
					"required": []string{"codes"},
					"properties": map[string]interface{}{
						"codes": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}
	_ = json.NewEncoder(w).Encode(tools)
}
