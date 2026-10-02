package main

import (
	"log"
	"net/http"

	"server/internal/config"

	"github.com/gin-gonic/gin"
)

func main() {
	app, err := config.Init()
	if err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}
	defer app.Close()

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Chat server is running smoothly!",
		})
	})

	log.Printf("Server is running on port :%s...", app.Port)

	if err := r.Run(":" + app.Port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
