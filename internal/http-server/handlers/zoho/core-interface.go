package zoho

type Core interface {
	GetOrderProducts(orderId, smartSenderId string) error
}
