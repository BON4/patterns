package middleware

import (
	"net/http"
	"strconv"
	"time"

	ratelimit "github.com/BON4/patterns/rate-limit"
	"github.com/BON4/patterns/server/internal/infra/redisclient"
	"github.com/BON4/patterns/server/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

func MetricsMiddleware(me telemetry.MetricExporter) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		me.SendRequestMetric(telemetry.RequestMetric{
			Method:    c.Request.Method,
			Status:    strconv.Itoa(c.Writer.Status()),
			Path:      c.FullPath(),
			StartTime: start,
		})
	}
}

func TrackUserRequests(userKvRepo *redisclient.UserKvRepo) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		err := userKvRepo.SaveUserRequest(ctx, ctx.ClientIP())
		if err != nil {
			logrus.Errorf("failed to save request for %s: %s", ctx.ClientIP(), err)
		}

		ctx.Next()
	}
}

func LoggingMiddleware(lg *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)

		lg.WithFields(logrus.Fields{
			"status":    c.Writer.Status(),
			"method":    c.Request.Method,
			"path":      c.Request.URL.Path,
			"ip":        c.ClientIP(),
			"latency":   latency.String(),
			"userAgent": c.Request.UserAgent(),
		}).Info("request completed")
	}
}

func RateLimitMiddleware(lg *logrus.Logger, rdb *redis.Client, rpm int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		l, err := ratelimit.GetTokenBucketLimiter(ip, &ratelimit.RateLimiterParams{
			Ctx:     c.Request.Context(),
			RClient: rdb,
			Rpm:     int64(rpm),
		})
		if err != nil {
			lg.WithError(err).Error("Failed to get rate limiter")
			c.Next()
			return
		}

		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			return
		}

		c.Next()
	}
}
