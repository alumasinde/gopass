package main

import (
	"github.com/alumasinde/passnow/internal/app"
	"github.com/alumasinde/passnow/internal/app/config"
	"log"
)

func main() {
	cfg := config.Load()
	a, err := app.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
