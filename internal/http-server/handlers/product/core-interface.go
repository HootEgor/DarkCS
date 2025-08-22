package product

import "DarkCS/entity"

type Core interface {
	FindProduct(article string) (interface{}, error)
	ProductsInfo(articles []string) ([]entity.ProductInfo, error)
}
