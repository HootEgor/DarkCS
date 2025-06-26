package entity

type Basket struct {
	UserUUID string         `json:"user_uuid" bson:"user_uuid"`
	Products []OrderProduct `json:"products" bson:"products"`
}
