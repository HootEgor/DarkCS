package product

type Core interface {
	FindProduct(article string) (interface{}, error)
	ProductsInfo(articles []string) (interface{}, error)
}
