package entity

type Assistant struct {
	Name           string   `json:"name"`
	Id             string   `json:"id"`
	Active         bool     `json:"active"`
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	VectorStoreId  string   `json:"vector_store_id"`
	ResponseFormat string   `json:"response_format"`
	AllowedTools   []string `json:"allowed_tools"`
}

const (
	OverseerAss     = "Overseer"
	ConsultantAss   = "Consultant"
	CalculatorAss   = "Calculator"
	OrderManagerAss = "Order Manager"
)

func GetResponseFormat(name string) interface{} {
	switch name {
	case "response_code":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"response": map[string]interface{}{
					"type":        "string",
					"description": "Contains the full user-facing response. Must NOT include any product codes (numerical identifiers).",
				},
				"codes": map[string]interface{}{
					"type":        "array",
					"description": "Contains an array of product codes (9-digit numbers) that were mentioned in the response, but the codes themselves MUST NOT appear in the \"response\" field.",
					"items": map[string]interface{}{
						"type":    "string",
						"pattern": "^\\d{9}$",
					},
				},
				"show_codes": map[string]interface{}{
					"type":        "boolean",
					"description": "Indicates whether the backend should utilize or display the associated product codes from the \"codes\" field in a follow-up message or interface. This flag does not affect the current \"response\", which must never include the codes directly.",
				},
			},
			"required":             []string{"response", "codes", "show_codes"},
			"additionalProperties": false,
		}
	case "response_assistant":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"assistant": map[string]interface{}{
					"type":        "string",
					"description": "The name of the assistant.",
					"enum": []string{
						ConsultantAss,
						CalculatorAss,
						OrderManagerAss,
					},
				},
			},
			"required":             []string{"assistant"},
			"additionalProperties": false,
		}
	default:
		return nil
	}
}

type ResponseCode struct {
	Response  string   `json:"response"`
	Codes     []string `json:"codes"`
	ShowCodes bool     `json:"show_codes"`
}

type ResponseAssistant struct {
	Assistant string `json:"assistant"`
}
