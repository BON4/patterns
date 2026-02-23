package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/BON4/patterns/server/internal/infra"
	"github.com/gin-gonic/gin"
)

type UserService interface {
	SyncToMongo(ctx context.Context) error
}

func (h *Handler) dumpRequests(c *gin.Context) {
	if err := h.userService.SyncToMongo(c.Request.Context()); err != nil {
		if errors.Is(err, infra.ErrLockNotAcquired) {
			c.JSON(http.StatusAccepted, gin.H{"status": "already_processing"})
			return
		}
		h.logger.WithError(err).Error("failed to dump user requests")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}
