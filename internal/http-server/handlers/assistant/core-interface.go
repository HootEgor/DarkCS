package assistant

import "DarkCS/entity"

type Core interface {
	AttachNewFile() error
	UpdateAssistant(name, id string, active bool, model, prompt, vectorStoreId string, responseFormat interface{}, allowedTools []string) error
	GetAllAssistants() ([]entity.Assistant, error)
}
