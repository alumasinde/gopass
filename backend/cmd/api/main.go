package main

import (
	"log"

	"github.com/alumasinde/gopass/internal/app"
	"github.com/alumasinde/gopass/internal/app/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	a, err := app.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
