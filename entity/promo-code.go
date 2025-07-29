package entity

type PromoCode struct {
	Code      string `json:"code" bson:"code"`
	Activated bool   `json:"activated" bson:"activated"`
}
