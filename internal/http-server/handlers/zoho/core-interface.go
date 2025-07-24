package zoho

type Core interface {
	GetOrderProducts(orderId string) (string, error)
}
