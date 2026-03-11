package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	tb "github.com/tucnak/telebot"
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

	bot.Handle("/start", func(m *tb.Message) {

		locBtn := tb.ReplyButton{
			Text:     "Отправить геолокацию",
			Location: true,
		}

		markup := &tb.ReplyMarkup{
			ResizeReplyKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{locBtn},
			},
		}

		if _, err := bot.Send(m.Sender, "Отправь геолоквцию:", markup); err != nil {
			log.Println("Ошибка отправки сообщения:", err)
		}
	})

	bot.Handle(tb.OnLocation, func(m *tb.Message) {

		if m.Location == nil {
			return
		}

		lat := m.Location.Lat
		lon := m.Location.Lng

		msg := fmt.Sprintf(
			"Результат\n\nШирота: %.6f\nДолгота: %.6f",
			lat,
			lon,
		)

		if _, err := bot.Send(m.Sender, msg); err != nil {
			log.Println("Ошибка отправки сообщения:", err)
		}
	})

	bot.Start()
}
