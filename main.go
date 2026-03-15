package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	tb "gopkg.in/telebot.v3"
)

type PrayerResponse struct {
	Data struct {
		Timings struct {
			Fajr    string `json:"Fajr"`
			Sunrise string `json:"Sunrise"`
			Dhuhr   string `json:"Dhuhr"`
			Asr     string `json:"Asr"`
			Maghrib string `json:"Maghrib"`
			Isha    string `json:"Isha"`
		} `json:"timings"`
	} `json:"data"`
}

var methods = map[string]int{
	"Muslim World League (Мир)": 3,
	"Umm Al-Qura (Аравия)":      4,
	"Egyptian (Африка)":         5,
	"Karachi (Азия)":            1,
	"Diyanet (Турция, Европа)":  13,
	"ISNA (Америка)":            2,
}

func lastThirdOfNight(maghribStr, fajrStr string) string {

	layout := "15:04"

	maghrib, _ := time.Parse(layout, maghribStr)
	fajr, _ := time.Parse(layout, fajrStr)

	// если фаджр уже следующего дня
	if fajr.Before(maghrib) {
		fajr = fajr.Add(24 * time.Hour)
	}

	nightDuration := fajr.Sub(maghrib)

	lastThirdStart := fajr.Add(-nightDuration / 3)

	return lastThirdStart.Format(layout)
}

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

	bot.SetCommands([]tb.Command{
		{Text: "start", Description: "Запустить бота"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "madhab", Description: "Выбрать мазхаб"},
		{Text: "method", Description: "Метод расчёта"},
	})

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

	bot.Handle("/madhab", func(c tb.Context) error {

		btnHanafi := tb.ReplyButton{Text: "Ханафи"}
		btnShafi := tb.ReplyButton{Text: "Шафии"}

		markup := &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btnHanafi, btnShafi},
			},
		}

		return c.Send("Выберите мазхаб:", markup)
	})

	bot.Handle("Ханафи", func(c tb.Context) error {

		chatID := c.Sender().ID

		_, err := conn.Exec(context.Background(),
			`UPDATE users SET school = 1 WHERE chat_id=$1`,
			chatID,
		)

		if err != nil {
			log.Println(err)
		}

		remove := &tb.ReplyMarkup{
			RemoveKeyboard: true,
		}

		return c.Send("Выбран ханафитский мазхаб", remove)
	})

	bot.Handle("Шафии", func(c tb.Context) error {

		chatID := c.Sender().ID

		_, err := conn.Exec(context.Background(),
			`UPDATE users SET school = 0 WHERE chat_id=$1`,
			chatID,
		)

		if err != nil {
			log.Println(err)
		}

		remove := &tb.ReplyMarkup{
			RemoveKeyboard: true,
		}

		return c.Send("Выбран шафиитский мазхаб", remove)
	})

	bot.Handle("/method", func(c tb.Context) error {

		btn1 := tb.ReplyButton{Text: "Muslim World League (Мир)"}
		btn2 := tb.ReplyButton{Text: "Umm Al-Qura (Аравия)"}
		btn3 := tb.ReplyButton{Text: "Egyptian (Африка)"}
		btn4 := tb.ReplyButton{Text: "Karachi (Азия)"}
		btn5 := tb.ReplyButton{Text: "Diyanet (Турция, Европа)"}
		btn6 := tb.ReplyButton{Text: "ISNA (Америка)"}

		markup := &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btn1, btn2},
				{btn3, btn4},
				{btn5, btn6},
			},
		}

		return c.Send("Выберите организацию расчёта:", markup)
	})

	bot.Handle(tb.OnText, func(c tb.Context) error {

		methodID, ok := methods[c.Text()]
		if !ok {
			return nil
		}

		chatID := c.Sender().ID

		_, err := conn.Exec(context.Background(),
			`UPDATE users SET method=$1 WHERE chat_id=$2`,
			methodID, chatID,
		)

		if err != nil {
			log.Println(err)
		}

		remove := &tb.ReplyMarkup{
			RemoveKeyboard: true,
		}

		return c.Send("Метод расчёта обновлён", remove)
	})

	bot.Handle("/today", func(c tb.Context) error {

		chatID := c.Sender().ID

		var lat float64
		var lon float64
		var school int
		var method int

		err := conn.QueryRow(context.Background(),
			`SELECT latitude, longitude, school, method FROM users WHERE chat_id=$1`,
			chatID,
		).Scan(&lat, &lon, &school, &method)

		if err != nil {
			return c.Send("Сначала отправьте геолокацию через /start")
		}

		url := fmt.Sprintf(
			"https://api.aladhan.com/v1/timings?latitude=%f&longitude=%f&method=%d&school=%d",
			lat, lon, method, school,
		)

		resp, err := http.Get(url)
		if err != nil {
			return c.Send("Ошибка получения данных намаза")
		}
		defer resp.Body.Close()

		var prayer PrayerResponse

		err = json.NewDecoder(resp.Body).Decode(&prayer)
		if err != nil {
			return c.Send("Ошибка обработки ответа API")
		}

		lastThird := lastThirdOfNight(
			prayer.Data.Timings.Maghrib,
			prayer.Data.Timings.Fajr,
		)

		msg := fmt.Sprintf(
			"Расписание на сегодня:\n\n"+
				"Фаджр: %s\n"+
				"Восход: %s\n"+
				"Зухр: %s\n"+
				"Аср: %s\n"+
				"Магриб: %s\n"+
				"Иша: %s\n\n"+
				"Последняя треть ночи: %s",
			prayer.Data.Timings.Fajr,
			prayer.Data.Timings.Sunrise,
			prayer.Data.Timings.Dhuhr,
			prayer.Data.Timings.Asr,
			prayer.Data.Timings.Maghrib,
			prayer.Data.Timings.Isha,
			lastThird,
		)

		return c.Send(msg)
	})

	bot.Start()
}
