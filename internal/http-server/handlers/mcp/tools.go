package mcp

func ToolsDescription() map[string]interface{} {
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

	return tools
}
