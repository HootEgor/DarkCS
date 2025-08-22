package assistant

import "DarkCS/entity"

type Core interface {
	AttachNewFile() error
	UpdateAssistant(name, id string, active bool, model, prompt, vectorStoreId, responseFormat string, allowedTools []string) error
	GetAllAssistants() ([]entity.Assistant, error)
}
