package mcp

type Core interface {
	Ping() string
	ProductsInfo(articles []string) (interface{}, error)
}
