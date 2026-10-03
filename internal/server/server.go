package server

import (
	"log"

	"server/internal/auth"
	"server/internal/config"

	"github.com/gin-gonic/gin"
)

type Server struct {
	router *gin.Engine
	cfg    *config.AppConfig
}

func NewServer(cfg *config.AppConfig) *Server {
	router := gin.Default()

	authRepo := auth.NewRepository(cfg.DB, cfg.Redis)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authService, cfg.DB, cfg.Redis)

	api := router.Group("/api")
	{
		authHandler.RegisterAuthRoutes(api.Group("/auth"))

		chat := api.Group("/chat")
		chat.Use(authHandler.Middleware())
		// chatHandler.RegisterRoutes(chat)
	}

	return &Server{router: router, cfg: cfg}
}

func (s *Server) Start() error {
	log.Printf("Server is starting on port :%s...", s.cfg.Port)
	return s.router.Run(":" + s.cfg.Port)
}
