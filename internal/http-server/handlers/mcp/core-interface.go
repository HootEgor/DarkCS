package mcp

import "DarkCS/entity"

type Core interface {
	Ping() string
	ProductsInfo(articles []string) ([]entity.ProductInfo, error)
}
