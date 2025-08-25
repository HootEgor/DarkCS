package mcp

import "encoding/json"

type Core interface {
	Ping() string
	HandleCommand(userUID, name string, args json.RawMessage) (interface{}, error)
}
