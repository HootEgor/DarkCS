package assistant

import "DarkCS/entity"

type Core interface {
	AttachNewFile() error
	UpdateAssistant(name, id string, active bool) error
	GetAllAssistants() ([]entity.Assistant, error)
}
