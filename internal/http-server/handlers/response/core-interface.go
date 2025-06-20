package response

import "DarkCS/entity"

type Core interface {
	ComposeResponse(msg entity.HttpUserMsg) (interface{}, error)
}
