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
