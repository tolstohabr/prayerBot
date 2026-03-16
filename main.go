package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
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

func showMadhabMenu(c tb.Context) error {

	btnHanafi := tb.ReplyButton{Text: "Ханафи"}
	btnShafi := tb.ReplyButton{Text: "Шафии"}

	markup := &tb.ReplyMarkup{
		ResizeKeyboard: true,
		ReplyKeyboard: [][]tb.ReplyButton{
			{btnHanafi, btnShafi},
		},
	}

	return c.Send("Выберите мазхаб:", markup)
}

func showMethodMenu(c tb.Context) error {

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
}

func roundCoord(v float64) float64 {
	return math.Round(v*100) / 100
}

func getOrCreateLocation(ctx context.Context, conn *pgx.Conn, lat, lon float64) (int, error) {

	var id int

	err := conn.QueryRow(ctx,
		`INSERT INTO locations (lat, lon)
		 VALUES ($1, $2)
		 ON CONFLICT (lat, lon)
		 DO UPDATE SET lat = EXCLUDED.lat
		 RETURNING id`,
		lat, lon,
	).Scan(&id)

	return id, err
}

func getOrCreateProfile(ctx context.Context, conn *pgx.Conn, locationID, method, school int) (int, error) {

	var id int

	err := conn.QueryRow(ctx,
		`INSERT INTO prayer_profiles (location_id, method, school)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (location_id,method,school)
		 DO UPDATE SET method = EXCLUDED.method
		 RETURNING id`,
		locationID, method, school,
	).Scan(&id)

	return id, err
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
		{Text: "settings", Description: "Настройки"},
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

	bot.Handle("/settings", func(c tb.Context) error {

		btnProfile := tb.ReplyButton{Text: "Профиль"}
		btnMadhab := tb.ReplyButton{Text: "Мазхаб"}
		btnMethod := tb.ReplyButton{Text: "Метод расчёта"}
		btnLocation := tb.ReplyButton{Text: "Геолокация"}
		btnSub := tb.ReplyButton{Text: "Подписка"}

		markup := &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btnProfile},
				{btnMadhab, btnMethod},
				{btnLocation},
				{btnSub},
			},
		}

		return c.Send("Настройки", markup)
	})

	bot.Handle("Мазхаб", showMadhabMenu)

	bot.Handle("Метод расчёта", showMethodMenu)

	bot.Handle("Геолокация", func(c tb.Context) error {

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

		return c.Send("Отправь новую геолокацию:", markup)
	})

	bot.Handle("/madhab", showMadhabMenu)
	bot.Handle("/method", showMethodMenu)

	bot.Handle(tb.OnLocation, func(c tb.Context) error {

		loc := c.Message().Location
		if loc == nil {
			return nil
		}

		lat := roundCoord(float64(loc.Lat))
		lon := roundCoord(float64(loc.Lng))

		chatID := c.Sender().ID

		ctx := context.Background()

		locationID, err := getOrCreateLocation(ctx, conn, lat, lon)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка сохранения локации")
		}

		method := 3
		school := 1

		profileID, err := getOrCreateProfile(ctx, conn, locationID, method, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка создания профиля")
		}

		_, err = conn.Exec(ctx,
			`INSERT INTO users (chat_id, profile_id)
		 VALUES ($1,$2)
		 ON CONFLICT (chat_id)
		 DO UPDATE SET profile_id = EXCLUDED.profile_id`,
			chatID, profileID,
		)

		if err != nil {
			log.Println(err)
		}

		msg := fmt.Sprintf(
			"Геолокация сохранена\n\nШирота: %.2f\nДолгота: %.2f",
			lat,
			lon,
		)

		remove := &tb.ReplyMarkup{
			RemoveKeyboard: true,
		}

		return c.Send(msg, remove)
	})

	bot.Handle("Подписка", func(c tb.Context) error {

		btnOn := tb.ReplyButton{Text: "Подписаться"}
		btnOff := tb.ReplyButton{Text: "Отписаться"}

		markup := &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btnOn, btnOff},
			},
		}

		return c.Send("Управление уведомлениями", markup)
	})

	bot.Handle("Подписаться", func(c tb.Context) error {

		chatID := c.Sender().ID

		_, err := conn.Exec(context.Background(),
			`UPDATE users SET subscribed=true WHERE chat_id=$1`,
			chatID,
		)

		if err != nil {
			log.Println(err)
			return c.Send("Ошибка подписки")
		}

		return c.Send("Вы подписались на уведомления о намазе")
	})

	bot.Handle("Отписаться", func(c tb.Context) error {

		chatID := c.Sender().ID

		_, err := conn.Exec(context.Background(),
			`UPDATE users SET subscribed=false WHERE chat_id=$1`,
			chatID,
		)

		if err != nil {
			log.Println(err)
			return c.Send("Ошибка отписки")
		}

		return c.Send("Вы отписались от уведомлений")
	})

	bot.Handle("Профиль", func(c tb.Context) error {

		chatID := c.Sender().ID

		var school int
		var method int
		var subscribed bool

		err := conn.QueryRow(context.Background(),
			`SELECT p.school, p.method, u.subscribed
	 FROM users u
	 JOIN prayer_profiles p ON u.profile_id = p.id
	 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&school, &method, &subscribed)

		if err != nil {
			return c.Send("Сначала отправьте геолокацию через /start")
		}

		madhab := "Шафии"
		if school == 1 {
			madhab = "Ханафи"
		}

		methodName := "Неизвестно"
		for name, id := range methods {
			if id == method {
				methodName = name
				break
			}
		}

		subStatus := "Нет"
		if subscribed {
			subStatus = "Да"
		}

		msg := fmt.Sprintf(
			"Профиль\n\n"+
				"Мазхаб: %s\n"+
				"Метод: %s\n"+
				"Подписка: %s",
			madhab,
			methodName,
			subStatus,
		)

		return c.Send(msg)
	})

	bot.Handle("Ханафи", func(c tb.Context) error {

		chatID := c.Sender().ID
		ctx := context.Background()

		var locationID int
		var method int

		err := conn.QueryRow(ctx,
			`SELECT p.location_id, p.method
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&locationID, &method)

		if err != nil {
			return c.Send("Сначала отправьте геолокацию")
		}

		school := 1

		profileID, err := getOrCreateProfile(ctx, conn, locationID, method, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка обновления мазхаба")
		}

		_, err = conn.Exec(ctx,
			`UPDATE users SET profile_id=$1 WHERE chat_id=$2`,
			profileID, chatID,
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
		ctx := context.Background()

		var locationID int
		var method int

		err := conn.QueryRow(ctx,
			`SELECT p.location_id, p.method
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&locationID, &method)

		if err != nil {
			return c.Send("Сначала отправьте геолокацию")
		}

		school := 0

		profileID, err := getOrCreateProfile(ctx, conn, locationID, method, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка обновления мазхаба")
		}

		_, err = conn.Exec(ctx,
			`UPDATE users SET profile_id=$1 WHERE chat_id=$2`,
			profileID, chatID,
		)

		if err != nil {
			log.Println(err)
		}

		remove := &tb.ReplyMarkup{
			RemoveKeyboard: true,
		}

		return c.Send("Выбран шафиитский мазхаб", remove)
	})

	bot.Handle(tb.OnText, func(c tb.Context) error {

		methodID, ok := methods[c.Text()]
		if !ok {
			return nil
		}

		chatID := c.Sender().ID
		ctx := context.Background()

		var locationID int
		var school int

		err := conn.QueryRow(ctx,
			`SELECT p.location_id, p.school
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&locationID, &school)

		if err != nil {
			return nil
		}

		profileID, err := getOrCreateProfile(ctx, conn, locationID, methodID, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка обновления метода")
		}

		_, err = conn.Exec(ctx,
			`UPDATE users SET profile_id=$1 WHERE chat_id=$2`,
			profileID, chatID,
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
			`SELECT l.lat, l.lon, p.method, p.school
	 FROM users u
	 JOIN prayer_profiles p ON u.profile_id = p.id
	 JOIN locations l ON p.location_id = l.id
	 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&lat, &lon, &method, &school)

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
