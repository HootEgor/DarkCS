package mcp

func ToolsDescription() map[string]interface{} {
	tools := map[string]interface{}{
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
					"type": "array",
					"items": map[string]interface{}{
						"type":     "object",
						"required": []string{"code", "name", "price", "url"},
						"properties": map[string]interface{}{
							"code": map[string]interface{}{
								"type":        "string",
								"description": "The unique product code.",
							},
							"name": map[string]interface{}{
								"type":        "string",
								"description": "The full name of the product.",
							},
							"price": map[string]interface{}{
								"type":        "number",
								"description": "The price of the product.",
							},
							"url": map[string]interface{}{
								"type":        "string",
								"description": "A URL to the product's image or page.",
							},
						},
					},
				},
			},
		},
	}

	return tools
}
