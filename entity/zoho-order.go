package entity

type ZohoOrder struct {
	ContactName        ContactName     `json:"Contact_Name"`
	ContactFullName    string          `json:"A0887ffac4e3b73fb6168af580709fd74"`
	ContactPhone       string          `json:"A737a105054835f9641fb492dda0c26c3"`
	ContactEmail       string          `json:"A4fe667f6f70c0892ac7e10ed209260b8"`
	ShippingAddress    string          `json:"A0d3aa57fb7d0fc67725ca891b3965663"`
	ShippingCountry    string          `json:"A68fdec5b7ce138314daea92f2d691979"`
	OrderedItems       []OrderedItem   `json:"Ordered_Items"`
	Discount           float64         `json:"Discount"`
	Description        string          `json:"Description"`
	CustomerNo         string          `json:"Customer_No"`
	ShippingState      string          `json:"Shipping_State"`
	Tax                float64         `json:"Tax"`
	BillingCountry     string          `json:"Billing_Country"`
	Carrier            string          `json:"Carrier"`
	Status             string          `json:"Status"`
	SalesCommission    float64         `json:"Sales_Commission"`
	DueDate            string          `json:"Due_Date"`
	BillingStreet      string          `json:"Billing_Street"`
	Adjustment         float64         `json:"Adjustment"`
	TermsAndConditions string          `json:"Terms_and_Conditions"`
	BillingCode        string          `json:"Billing_Code"`
	ProductDetails     []ProductDetail `json:"Product_Details,omitempty"`
	Location           string          `json:"Location_DR"`
	OrderSource        string          `json:"Order_Source"`
	Subject            string          `json:"Subject"`
}

type ContactName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type OrderedItem struct {
	Product   ZohoProduct `json:"Product_Name"`
	Quantity  int         `json:"Quantity"`
	Discount  float64     `json:"Discount"`
	DiscountP float64     `json:"DiscountP"`
	ListPrice float64     `json:"List_Price"`
	Total     float64     `json:"Total"`
}

type ZohoProduct struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id"`
}

type ProductDetail struct {
	Product     ProductID `json:"product"`
	Quantity    int       `json:"quantity"`
	Discount    float64   `json:"Discount"`
	ProductDesc string    `json:"product_description"`
	UnitPrice   float64   `json:"Unit Price"`
	LineTax     []LineTax `json:"line_tax"`
}

type ProductID struct {
	ID string `json:"id"`
}

type LineTax struct {
	Percentage float64 `json:"percentage"`
	Name       string  `json:"name"`
}

type OrderStatus struct {
	Status string `json:"Status"`
}

// OrderDetail contains detailed information about an order.
type OrderDetail struct {
	ID          string      `json:"id"`
	Subject     string      `json:"Subject"`
	Status      string      `json:"Status"`
	ContactName ContactName `json:"Contact_Name"`
	TTN         string      `json:"Aa2e053928236368ec7865f3558a58c4f"`
}

// ServiceRating represents a service rating to be sent to Zoho CRM.
type ServiceRating struct {
	OrderNumber   string `json:"Sales_Orders_raiting"`
	ContactID     string `json:"Contact_raiting"`
	ServiceRating int    `json:"Servise_rating"`
}

// IsActive returns true if the order is in an active status.
func (o *OrderDetail) IsActive() bool {
	return o.Status == OrderStatusNew ||
		o.Status == OrderStatusProcessing ||
		o.Status == OrderStatusInvoiced
}

const (
	OrderStatusNew        = "Нове"
	OrderStatusProcessing = "Оброблення замовлення"
	OrderStatusInvoiced   = "Рахунок виставлено"
)
