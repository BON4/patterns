package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/BON4/patterns/server/internal/domain"
	"github.com/BON4/patterns/server/internal/exchange"
	"github.com/BON4/patterns/server/internal/geo"
	"github.com/BON4/patterns/server/internal/infra"
)

type GeoResolver interface {
	GetLocationByIP(ip string) (*geo.IpLocation, error)
}

type ProductRepo interface {
	GetProduct(ctx context.Context, name string) (*domain.Product, error)
}

type ExchangeClient interface {
	GetRates(ctx context.Context) ([]exchange.Rate, error)
}

type ProductService struct {
	repo     ProductRepo
	exchange ExchangeClient
	geo      GeoResolver
	stampede infra.RedisStampede[[]exchange.Rate]
}

func NewProductService(
	repo ProductRepo,
	exchange ExchangeClient,
	geo GeoResolver,
	stampede infra.RedisStampede[[]exchange.Rate],
) *ProductService {
	return &ProductService{repo: repo, exchange: exchange, geo: geo, stampede: stampede}
}

type ProductResponse struct {
	Name           string  `json:"name"`
	PriceUSD       float64 `json:"price_usd"`
	PriceConverted float64 `json:"price_converted"`
}

func (s *ProductService) GetProduct(ctx context.Context, name, ip string) (*ProductResponse, error) {
	p, err := s.repo.GetProduct(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if p == nil {
		return nil, nil
	}

	ipLoc, err := s.geo.GetLocationByIP(ip)
	if err != nil {
		return nil, fmt.Errorf("get ip location: %w", err)
	}

	rates, err := s.stampede.Get(ctx, func(ctx context.Context) (*[]exchange.Rate, error) {
		time.Sleep(time.Second * 5)
		rates, err := s.exchange.GetRates(ctx)
		if err != nil {
			return nil, fmt.Errorf("get rates: %w", err)
		}

		return &rates, nil
	})
	if err != nil {
		if errors.Is(err, infra.ErrStampedeTimeout) {
			// TODO: maybe add retry
			return nil, err
		}
		return nil, err
	}

	if rates == nil {
		return &ProductResponse{
			Name:     p.Name,
			PriceUSD: p.Price,
		}, nil
	}

	rateIdx := slices.IndexFunc(*rates, func(r exchange.Rate) bool {
		if strings.ToLower(r.Country) == strings.ToLower(ipLoc.Country) {
			return true
		}
		return false
	})

	return &ProductResponse{
		Name:           p.Name,
		PriceUSD:       p.Price,
		PriceConverted: p.Price * (*rates)[rateIdx].Price,
	}, nil
}
