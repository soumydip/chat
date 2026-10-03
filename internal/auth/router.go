package auth

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	service   *Service
	db        *pgxpool.Pool
	rdb       *redis.Client
	jwtSecret string
}

func NewHandler(service *Service, db *pgxpool.Pool, rdb *redis.Client) *Handler {
	return &Handler{
		service:   service,
		db:        db,
		rdb:       rdb,
		jwtSecret: os.Getenv("JWT_SECRET"),
	}
}

func (h *Handler) Middleware() gin.HandlerFunc {
	return ValidateJWT(h.db, h.rdb, h.jwtSecret)
}

func (h *Handler) RegisterAuthRoutes(r *gin.RouterGroup) {
	// public
	r.POST("/register", h.RegisterUser)
	r.POST("/login", h.LoginUser)
	r.POST("/logout", h.Logout)

	// protected
	protected := r.Group("")
	protected.Use(h.Middleware())
	protected.GET("/me", h.GetCurrentUser)
}


