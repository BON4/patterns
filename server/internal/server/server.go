package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/BON4/patterns/server/internal/config"
	"github.com/BON4/patterns/server/internal/handlers"
	"github.com/BON4/patterns/server/internal/middleware"
	"github.com/BON4/patterns/server/internal/repo"
	"github.com/BON4/patterns/server/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

type Server struct {
	cfg           config.ServerConfig
	logger        *logrus.Logger
	engine        *gin.Engine
	srv           *http.Server
	shutDownQueue []func(context.Context) error
}

func NewServer(
	cfg config.ServerConfig,
	h *handlers.Handler,
	lg *logrus.Logger,
	rdb *redis.Client,
	prometheusMetricExporter telemetry.MetricExporter,
) *Server {
	g := gin.New()

	s := &Server{
		cfg:    cfg,
		logger: lg,
		engine: g,
	}

	s.setupMiddleware(prometheusMetricExporter, rdb)

	s.srv = &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      s.engine,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	h.Register(s.engine)

	return s
}

func (s *Server) setupMiddleware(pm telemetry.MetricExporter, rdb *redis.Client) {
	s.engine.Use(middleware.TrackUserRequests(repo.NewUserKvRepo(rdb)))
	s.engine.Use(middleware.MetricsMiddleware(pm))
	s.engine.Use(otelgin.Middleware(s.cfg.ServiceName))
	s.engine.Use(middleware.LoggingMiddleware(s.logger))
	s.engine.Use(middleware.RateLimitMiddleware(s.logger, rdb, s.cfg.RateLimiter.RPM))

	s.engine.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		s.logger.WithFields(logrus.Fields{"path": c.Request.URL.Path, "method": c.Request.Method}).Errorf("panic recovered: %v", err)
		c.AbortWithStatus(500)
	}))

	s.engine.SetTrustedProxies([]string{"0.0.0.0/0"})
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		s.logger.Infof("starting server on %s", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
