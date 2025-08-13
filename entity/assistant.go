package entity

type Assistant struct {
	Name   string `json:"name"`
	Id     string `json:"id"`
	Active bool   `json:"active"`
}

const (
	OverseerAss     = "Overseer"
	ConsultantAss   = "Consultant"
	CalculatorAss   = "Calculator"
	OrderManagerAss = "Order Manager"
)
