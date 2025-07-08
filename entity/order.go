package entity

type Order struct {
	User     UserInfo       `json:"user" bson:"user"`
	Products []OrderProduct `json:"products" bson:"products"`
}
