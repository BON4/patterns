package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	productService ProductService
	userService    UserService
	logger         *logrus.Logger
}

func NewHandler(productService ProductService, userService UserService, logger *logrus.Logger) *Handler {
	return &Handler{
		productService: productService,
		userService:    userService,
		logger:         logger,
	}
}

func (h *Handler) Register(engine *gin.Engine) {
	engine.GET("/ping", h.ping)
	engine.GET("/products/:name", h.getProduct)
	engine.POST("/users/dump-requests", h.dumpRequests)
}
