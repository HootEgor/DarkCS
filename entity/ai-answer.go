package entity

type AiAnswer struct {
	Text      string        `json:"text" bson:"text"`
	Assistant string        `json:"assistant" bson:"assistant"`
	Products  []ProductInfo `json:"products" bson:"products"`
}
