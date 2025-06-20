package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"log/slog"
)

func (o *Overseer) handleCommand(name, args string) (interface{}, error) {
	o.log.With(
		slog.String("command", name),
		slog.String("args", args),
	).Debug("handling command")
	switch name {
	case "get_products_info":
		return o.handleGetProductInfo(args)
	default:
		return "", nil
	}
}

type getProductInfoResp struct {
	Codes []string `json:"codes"`
}

func (o *Overseer) handleGetProductInfo(args string) ([]entity.ProductInfo, error) {
	var resp *getProductInfoResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		o.log.With(
			slog.String("args", args),
			sl.Err(err),
		).Error("unmarshalling response")
		return nil, err
	}

	productsInfo, err := o.productService.GetProductInfo(resp.Codes)
	if err != nil {
		o.log.With(
			slog.Any("codes", resp.Codes),
			sl.Err(err),
		).Error("getting product info")
		return nil, err
	}

	return productsInfo, nil
}
