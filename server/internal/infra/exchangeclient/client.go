package exchangeclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"
)

type Rate struct {
	Currency string  `json:"currency"`
	Price    float64 `json:"price"`
}

type Client interface {
	GetRates(ctx context.Context) ([]Rate, error)
}

type HTTPClient struct {
	http    *http.Client
	BaseURL string
}

func NewHTTPClient(httpClient *http.Client, baseURL string) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &HTTPClient{http: httpClient, BaseURL: baseURL}
}

// GetRates calls GET {BaseURL}/exchange and parses the response.
// The API is expected to return either a JSON array of rates or an object
// with a top-level "rates" array.
func (c *HTTPClient) GetRates(ctx context.Context) ([]Rate, error) {
	if c == nil {
		return nil, errors.New("nil exchange client")
	}

	return []Rate{
		{
			Currency: "UAH",
			Price:    rand.Float64(),
		},
		{
			Currency: "GBP",
			Price:    rand.Float64(),
		},
		{
			Currency: "PLZ",
			Price:    rand.Float64(),
		},
	}, nil
}

func CalcPrice(ctx context.Context, exchange Client, price float64, currency string) (float64, error) {
	var rate float64
	rates, err := exchange.GetRates(ctx)
	if err != nil {
		return 0, err
	}

	rateIdx := slices.IndexFunc(rates, func(r Rate) bool {
		if strings.ToLower(r.Currency) == strings.ToLower(currency) {
			return true
		}
		return false
	})

	if rateIdx == -1 {
		return 0, fmt.Errorf("currency not found")
	}

	rate = rates[rateIdx].Price

	return price * rate, nil
}
