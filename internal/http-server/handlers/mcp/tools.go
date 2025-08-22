package mcp

func ToolsDescription() map[string]interface{} {
	tools := map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "get_products_info",
				"description": "Fetches information about products based on an array of product codes. It returns info only about available products, meaning if 3 codes are given but only 2 exist, the output includes only 2.",
				"parameters": map[string]interface{}{
					"type":     "object",
					"required": []string{"codes"},
					"properties": map[string]interface{}{
						"codes": map[string]interface{}{
							"type":        "array",
							"description": "Array of product codes to fetch information for",
							"items": map[string]interface{}{
								"type":        "string",
								"description": "Unique code for each product",
							},
						},
					},
					"additionalProperties": false,
				},
				"strict": true,
			},
		},
	}

	return tools
}
