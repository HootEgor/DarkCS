package mcp

func ToolsDescription() map[string]interface{} {
	return map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "get_products_info",
				"description": "Fetches information about products based on product codes",
				"inputSchema": map[string]interface{}{
					"type":     "object",
					"required": []string{"codes"},
					"properties": map[string]interface{}{
						"codes": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
				},
				"outputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"products": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"code":  map[string]interface{}{"type": "string"},
									"name":  map[string]interface{}{"type": "string"},
									"price": map[string]interface{}{"type": "number"},
									"url":   map[string]interface{}{"type": "string"},
								},
								"required": []string{"code", "name", "price"},
							},
						},
					},
					"required": []string{"products"},
				},
			},
		},
	}
}
