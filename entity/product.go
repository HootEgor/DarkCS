package entity

type Product struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Price string `json:"price"`
	Url   string `json:"url"`
}

type ProductInfo struct {
	Product     string `json:"product"`
	Group       string `json:"group"`
	Code        string `json:"code"`
	Description string `json:"description"`
}
