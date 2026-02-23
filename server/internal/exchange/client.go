package exchange

import (
	"context"
	"errors"
	"math/rand/v2"

	"github.com/BON4/patterns/server/internal/infra"
)

type Rate struct {
	Currency string  `json:"currency"`
	Country  string  `json:"country"`
	Price    float64 `json:"price"`
}

type ExchangeClient struct {
	http *infra.HTTPClient
}

func NewExchangeClient(http *infra.HTTPClient) *ExchangeClient {
	return &ExchangeClient{http: http}
}

func (c *ExchangeClient) GetRates(ctx context.Context) ([]Rate, error) {
	if c.http == nil {
		return nil, errors.New("nil exchange client")
	}

	return []Rate{
		{
			Currency: "UAH",
			Country:  "Ukraine",
			Price:    rand.Float64(),
		},
		{
			Currency: "GBP",
			Country:  "UK",
			Price:    rand.Float64(),
		},
		{
			Currency: "PLZ",
			Country:  "Poland",
			Price:    rand.Float64(),
		},
	}, nil
}
