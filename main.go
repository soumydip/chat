package main

import (
	"log"

	"server/internal/config"
	"server/internal/server"
)

func main() {
	cfg, err := config.Init()
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}
	defer cfg.Close()

	srv := server.NewServer(cfg)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}
