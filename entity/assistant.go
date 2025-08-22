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
