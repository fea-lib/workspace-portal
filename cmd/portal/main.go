package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"workspace-portal/internal/config"
	"workspace-portal/internal/server"
)

func main() {
	// CLI flags
	configPath := flag.String("config", "", "path to config.yaml")
	flag.Parse()

	// Resolve config path: flag > env var > default
	if *configPath == "" {
		if v := os.Getenv("PORTAL_CONFIG"); v != "" {
			*configPath = v
		} else {
			*configPath = "config.yaml"
		}
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Start server
	log.Printf("workspace-portal starting on :%d", cfg.PortalPort)
	if err := server.Start(cfg); err != nil {
		log.Fatal(err)
	}
}
