package handlers

import (
	"context"
	"net/http"

	"github.com/BON4/patterns/server/internal/service"
	"github.com/gin-gonic/gin"
)

type ProductService interface {
	GetProduct(ctx context.Context, name, ip string) (*service.ProductResponse, error)
}

func (h *Handler) getProduct(c *gin.Context) {
	name := c.Param("name")
	product, err := h.productService.GetProduct(c.Request.Context(), name, c.ClientIP())
	if err != nil {
		h.logger.WithError(err).Error("failed to get product")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if product == nil {
		h.logger.Warnf("failed to retrive product: %s, not found", name)
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": product})
}
