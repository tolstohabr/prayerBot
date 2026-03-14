package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	tb "gopkg.in/telebot.v3"
)

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Println(".env файл не найден")
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN не установлен")
	}

	bot, err := tb.NewBot(tb.Settings{
		Token:  botToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
	}

	bot.Handle("/start", func(c tb.Context) error {
		locBtn := tb.ReplyButton{
			Text:     "Отправить геолокацию",
			Location: true,
		}

		markup := &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{locBtn},
			},
		}

		return c.Send("Отправь геолокацию:", markup)
	})

	bot.Handle(tb.OnLocation, func(c tb.Context) error {

		loc := c.Message().Location
		if loc == nil {
			return nil
		}

		lat := loc.Lat
		lon := loc.Lng

		msg := fmt.Sprintf(
			"Результат\n\nШирота: %.6f\nДолгота: %.6f",
			lat,
			lon,
		)

		remove := &tb.ReplyMarkup{
			RemoveKeyboard: true,
		}

		return c.Send(msg, remove)
	})

	bot.Start()
}
