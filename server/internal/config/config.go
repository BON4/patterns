package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type RateLimiterConfig struct {
	RPM int `yaml:"rpm,omitempty"`
}

type ServerConfig struct {
	ServiceName     string            `yaml:"service-name"`
	Port            string            `yaml:"port"`
	RedisURL        string            `yaml:"redis-url"`
	MongoURI        string            `yaml:"mongo-uri"`
	MongoDB         string            `yaml:"mongo-db"`
	ExchangeBaseURL string            `yaml:"exchange-base-url"`
	RateLimiter     RateLimiterConfig `yaml:"rate-limiter,omitempty"`
}

func (c *ServerConfig) Validate() error {
	if c.ServiceName == "" {
		return errors.New("service-name is required")
	}

	if c.Port == "" {
		return errors.New("port is required")
	}

	port := strings.TrimPrefix(c.Port, ":")
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid port: %s", c.Port)
	}

	if c.RedisURL == "" {
		return errors.New("redis_url is required")
	}

	if _, err := redis.ParseURL(c.RedisURL); err != nil {
		return fmt.Errorf("invalid redis_url: %w", err)
	}

	if c.MongoURI == "" {
		return errors.New("mongo-uri is required")
	}

	if c.MongoDB == "" {
		return errors.New("mongo-db is required")
	}

	return nil
}
