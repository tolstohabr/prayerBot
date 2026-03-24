package main

import (
	"log"

	"prayerBot/internal/bot"
	"prayerBot/internal/config"
	"prayerBot/internal/repository"
	"prayerBot/internal/service"
)

func main() {
	cfg := config.Load()

	pool := repository.NewDB(cfg.DBUrl)
	defer pool.Close()

	repo := repository.New(pool)
	service := service.New(repo)

	b := bot.New(cfg.BotToken, service)

	log.Println("Бот запущен")
	b.Start()
}
