package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"time"

	cfg "github.com/BON4/patterns/server/internal/config"
	"github.com/BON4/patterns/server/internal/middleware"
	"github.com/BON4/patterns/server/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

type Server struct {
	cfg    cfg.ServerConfig
	logger *logrus.Logger
	rdb    *redis.Client

	engine                   *gin.Engine
	srv                      *stdhttp.Server
	shutDownQueue            []func(context.Context) error
	prometheusMetricExporter telemetry.MetricExporter
}

func NewServer(cfg cfg.ServerConfig, lg *logrus.Logger, rdb *redis.Client, prometheusMetricExporter telemetry.MetricExporter) *Server {
	g := gin.New()

	s := &Server{
		cfg:                      cfg,
		logger:                   lg,
		rdb:                      rdb,
		engine:                   g,
		prometheusMetricExporter: prometheusMetricExporter,
	}

	s.setup()

	s.srv = &stdhttp.Server{
		Addr:         ":" + cfg.Port,
		Handler:      s.engine,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return s
}

func (s *Server) setup() {
	s.engine.Use(middleware.MetricsMiddleware(s.prometheusMetricExporter))
	s.engine.Use(otelgin.Middleware(s.cfg.ServiceName))
	s.engine.Use(middleware.LoggingMiddleware(s.logger))
	s.engine.Use(middleware.RateLimitMiddleware(s.logger, s.rdb, s.cfg.RateLimiter.RPM))

	s.engine.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		s.logger.WithFields(logrus.Fields{"path": c.Request.URL.Path, "method": c.Request.Method}).Errorf("panic recovered: %v", err)
		c.AbortWithStatus(500)
	}))

	s.engine.SetTrustedProxies([]string{"0.0.0.0/0"})

	s.engine.GET("/ping", func(c *gin.Context) {
		c.JSON(stdhttp.StatusOK, gin.H{"message": "pong"})
	})

	s.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	s.engine.POST("/echo", func(c *gin.Context) {
		var json struct {
			User string `json:"user" binding:"required"`
		}
		if err := c.ShouldBindJSON(&json); err != nil {
			c.JSON(stdhttp.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(stdhttp.StatusOK, gin.H{"status": "hello", "recipient": json.User})
	})
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		s.logger.Infof("starting server on %s", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && err != stdhttp.ErrServerClosed {
			s.logger.WithError(err).Error("server start failed")
		}
	}()

	<-ctx.Done()
	return nil
}

func (s *Server) StartBlocking() error {
	s.logger.Infof("starting server on %s", s.srv.Addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	var shutDownErrors = []error{}
	for _, f := range s.shutDownQueue {
		if err := f(ctx); err != nil {
			shutDownErrors = append(shutDownErrors, err)
		}
	}
	return errors.Join(append(shutDownErrors, s.srv.Shutdown(ctx))...)
}
