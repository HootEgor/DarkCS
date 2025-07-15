package entity

type ProductInfo struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Price string `json:"price"`
	Url   string `json:"url"`
}

type Product struct {
	Product     string `json:"product"`
	Group       string `json:"group"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

type OrderProduct struct {
	Name      string `json:"name"`
	Price     string `json:"price"`
	Code      string `json:"code"`
	Discount  int    `json:"discount"`
	Quantity  int    `json:"quantity"`
	Available bool   `json:"available,omitempty"`
	ZohoId    string `json:"zoho_id,omitempty"`
}

func ProdForAssistant(products []OrderProduct) interface{} {
	result := make([]interface{}, len(products))
	for i, p := range products {
		result[i] = struct {
			Name     string `json:"name"`
			Price    string `json:"price"`
			Code     string `json:"code"`
			Quantity int    `json:"quantity"`
			Discount int    `json:"discount,omitempty"`
		}{
			Name:     p.Name,
			Price:    p.Price,
			Code:     p.Code,
			Quantity: p.Quantity,
			Discount: p.Discount,
		}
	}
	return result
}
