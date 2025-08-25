package mcp

func ToolsDescription(assName string) map[string]interface{} {
	// Define tool sets
	baseTools := []map[string]interface{}{
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
		},
	}

	shopTools := []map[string]interface{}{
		{
			"name":        "create_order",
			"description": "Procces confirmed order",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"additionalProperties": false,
				"required":             []string{},
			},
		},
		{
			"name":        "get_basket",
			"description": "Retrieves the current basket of products.",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"additionalProperties": false,
				"required":             []string{},
			},
		},
		{
			"name":        "add_to_basket",
			"description": "Adds products to the shopping basket, return modified basket",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":     "object",
				"required": []string{"products"},
				"properties": map[string]interface{}{
					"products": map[string]interface{}{
						"type":        "array",
						"description": "List of products to add to the basket",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"code": map[string]interface{}{
									"type":        "string",
									"description": "Unique code of the product",
								},
								"quantity": map[string]interface{}{
									"type":        "integer",
									"description": "Quantity of the product to add",
								},
							},
							"required":             []string{"code", "quantity"},
							"additionalProperties": false,
						},
					},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "remove_from_basket",
			"description": "Removes products from the shopping basket, return modified basket",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":     "object",
				"required": []string{"products"},
				"properties": map[string]interface{}{
					"products": map[string]interface{}{
						"type":        "array",
						"description": "List of products to remove from the basket",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"code": map[string]interface{}{
									"type":        "string",
									"description": "Unique code of the product",
								},
								"quantity": map[string]interface{}{
									"type":        "integer",
									"description": "Quantity of the product to remove",
								},
							},
							"required":             []string{"code", "quantity"},
							"additionalProperties": false,
						},
					},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "get_user_info",
			"description": "Retrieves the current user contact info.",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"additionalProperties": false,
				"required":             []string{},
			},
		},
		{
			"name":        "update_user_address",
			"description": "Update user address",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":     "object",
				"required": []string{"address"},
				"properties": map[string]interface{}{
					"address": map[string]interface{}{
						"type":        "string",
						"description": "User's address",
					},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "validate_order",
			"description": "Validate products in order",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"additionalProperties": false,
				"required":             []string{},
			},
		},
		{
			"name":        "clear_basket",
			"description": "Clear the current basket of products.",
			"strict":      true,
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"additionalProperties": false,
				"required":             []string{},
			},
		},
	}

	// Choose tools based on assName
	var tools []map[string]interface{}
	switch assName {
	case "Order Manager":
		tools = append(baseTools, shopTools...)
	default:
		tools = baseTools
	}

	return map[string]interface{}{
		"tools": tools,
	}
}
