package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BON4/patterns/server/internal/config"
	"github.com/BON4/patterns/server/internal/exchange"
	"github.com/BON4/patterns/server/internal/geo"
	"github.com/BON4/patterns/server/internal/handlers"
	"github.com/BON4/patterns/server/internal/infra"
	"github.com/BON4/patterns/server/internal/logger"
	"github.com/BON4/patterns/server/internal/repo"
	"github.com/BON4/patterns/server/internal/server"
	"github.com/BON4/patterns/server/internal/service"
	"github.com/BON4/patterns/server/internal/telemetry"
	yaml "github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var configPath string
	pflag.StringVarP(&configPath, "config", "c", "", "path to config file")
	pflag.Parse()

	if configPath == "" {
		log.Error("config not specified")
		os.Exit(1)
	}

	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	var cfg config.ServerConfig

	cfg.RateLimiter.RPM = 20

	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		log.Errorf("invalid config: %v", err)
		os.Exit(1)
	}

	lg := logger.New()

	rdb, err := infra.NewRedis(ctx, cfg.RedisURL)
	if err != nil {
		lg.WithError(err).Error("redis connect failed")
		os.Exit(1)
	}

	// MongoDB
	mc, err := infra.NewMongo(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		lg.WithError(err).Error("mongo connect failed")
		os.Exit(1)
	}
	defer mc.Close(ctx)

	rateStempide := infra.NewRedisStampede[[]exchange.Rate](rdb, "currenct-rate", time.Minute)

	pm, err := telemetry.RegisterPrometheusMetricsExporter()
	if err != nil {
		lg.WithError(err).Error("prometheus setup failed")
		os.Exit(1)
	}

	userRepo := repo.NewUserMongoRepo(mc.DB, "users")
	productRepo := repo.NewProductMongoRepo(mc.DB, "products")
	userKvRepo := repo.NewUserKvRepo(rdb)
	exchangeClient := exchange.NewExchangeClient(infra.NewHTTPClient(&http.Client{}, cfg.ExchangeBaseURL))

	// services
	userService := service.NewUserService(userKvRepo, userRepo)
	productService := service.NewProductService(productRepo, exchangeClient, &geo.LocalGeoResolver{}, *rateStempide)

	h := handlers.NewHandler(productService, userService, lg)
	srv := server.NewServer(cfg, h, lg, rdb, pm)

	if err := srv.StartBlocking(); err != nil {
		lg.WithError(err).Error("server start error")
	}

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(stopCtx); err != nil {
		lg.WithError(err).Error("shutdown error")
	}

	fmt.Println("Server stopped")
}
