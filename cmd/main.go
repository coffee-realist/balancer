package main

import (
	"github.com/coffee-realist/balancer/internal/config"
	"github.com/coffee-realist/balancer/internal/server"
	"log"
)

func main() {
	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := server.Start(cfg); err != nil {
		log.Fatalf("server error: %v", err)
	}

	log.Printf("starting load balancer on %s", cfg.ListenPort)
}
