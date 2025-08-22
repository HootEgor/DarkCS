package core

import (
	"DarkCS/entity"
	"fmt"
)

func (c *Core) FindProduct(article string) (interface{}, error) {
	if c.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	return c.ps.ProductSearch(article)
}

func (c *Core) ProductsInfo(articles []string) ([]entity.ProductInfo, error) {
	if c.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	return c.ps.GetProductInfo(articles)
}
