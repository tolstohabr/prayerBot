package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
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

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	conn, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("Ошибка подключения к базе:", err)
	}

	log.Println("Подключение к базе успешно")

	defer conn.Close(context.Background())

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

		chatID := c.Sender().ID

		_, err := conn.Exec(context.Background(),
			`INSERT INTO users (chat_id, latitude, longitude, subscribed)
		 VALUES ($1, $2, $3, FALSE)
		 ON CONFLICT (chat_id) DO UPDATE
		 SET latitude = EXCLUDED.latitude,
		     longitude = EXCLUDED.longitude`,
			chatID, lat, lon,
		)
		if err != nil {
			log.Println("Ошибка при вставке в БД:", err)
		}

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
