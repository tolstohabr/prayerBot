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

	"github.com/jackc/pgx/v5/pgxpool"
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

func getOrCreateLocation(ctx context.Context, pool *pgxpool.Pool, lat, lon float64) (int, error) {

	var id int

	err := pool.QueryRow(ctx,
		`INSERT INTO locations (lat, lon)
		 VALUES ($1, $2)
		 ON CONFLICT (lat, lon)
		 DO UPDATE SET lat = EXCLUDED.lat
		 RETURNING id`,
		lat, lon,
	).Scan(&id)

	return id, err
}

func getOrCreateProfile(ctx context.Context, pool *pgxpool.Pool, locationID, method, school int) (int, error) {

	var id int

	err := pool.QueryRow(ctx,
		`INSERT INTO prayer_profiles (location_id, method, school)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (location_id,method,school)
		 DO UPDATE SET method = EXCLUDED.method
		 RETURNING id`,
		locationID, method, school,
	).Scan(&id)

	return id, err
}

func savePrayerTimes(ctx context.Context, pool *pgxpool.Pool, profileID int, date time.Time, t PrayerResponse) error {

	_, err := pool.Exec(ctx,
		`INSERT INTO prayer_times
		(profile_id, date, fajr, sunrise, dhuhr, asr, maghrib, isha)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (profile_id, date) DO NOTHING`,
		profileID,
		date.Format("2006-01-02"),
		t.Data.Timings.Fajr,
		t.Data.Timings.Sunrise,
		t.Data.Timings.Dhuhr,
		t.Data.Timings.Asr,
		t.Data.Timings.Maghrib,
		t.Data.Timings.Isha,
	)

	return err
}

func getPrayerTimes(ctx context.Context, pool *pgxpool.Pool, profileID int, date time.Time) (PrayerResponse, bool) {

	var t PrayerResponse

	err := pool.QueryRow(ctx,
		`SELECT fajr, sunrise, dhuhr, asr, maghrib, isha
		 FROM prayer_times
		 WHERE profile_id=$1 AND date=$2`,
		profileID,
		date.Format("2006-01-02"),
	).Scan(
		&t.Data.Timings.Fajr,
		&t.Data.Timings.Sunrise,
		&t.Data.Timings.Dhuhr,
		&t.Data.Timings.Asr,
		&t.Data.Timings.Maghrib,
		&t.Data.Timings.Isha,
	)

	if err != nil {
		return t, false
	}

	return t, true
}

func formatTime(t string) string {
	if len(t) >= 5 {
		return t[:5]
	}
	return t
}

func sendPrayerNotifications(bot *tb.Bot, pool *pgxpool.Pool) {
	ctx := context.Background()

	now := time.Now()
	today := now.Format("2006-01-02")
	currentTime := now.Format("15:04:05")

	rows, err := pool.Query(ctx,
		`SELECT id, profile_id,
		        fajr, dhuhr, asr, maghrib, isha,
		        fajr_notified, dhuhr_notified, asr_notified,
		        maghrib_notified, isha_notified
		 FROM prayer_times
		 WHERE date=$1`,
		today,
	)
	if err != nil {
		log.Println("Ошибка запроса prayer_times:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {

		var id int
		var profileID int

		var fajr, dhuhr, asr, maghrib, isha string
		var fajrN, dhuhrN, asrN, maghribN, ishaN bool

		err := rows.Scan(
			&id, &profileID,
			&fajr, &dhuhr, &asr, &maghrib, &isha,
			&fajrN, &dhuhrN, &asrN, &maghribN, &ishaN,
		)
		if err != nil {
			continue
		}

		checkAndSend(bot, pool, profileID, "Фаджр", fajr, fajrN, currentTime,
			`UPDATE prayer_times SET fajr_notified=true WHERE id=$1`, id)

		checkAndSend(bot, pool, profileID, "Зухр", dhuhr, dhuhrN, currentTime,
			`UPDATE prayer_times SET dhuhr_notified=true WHERE id=$1`, id)

		checkAndSend(bot, pool, profileID, "Аср", asr, asrN, currentTime,
			`UPDATE prayer_times SET asr_notified=true WHERE id=$1`, id)

		checkAndSend(bot, pool, profileID, "Магриб", maghrib, maghribN, currentTime,
			`UPDATE prayer_times SET maghrib_notified=true WHERE id=$1`, id)

		checkAndSend(bot, pool, profileID, "Иша", isha, ishaN, currentTime,
			`UPDATE prayer_times SET isha_notified=true WHERE id=$1`, id)
	}
}

func checkAndSend(
	bot *tb.Bot,
	pool *pgxpool.Pool,
	profileID int,
	name string,
	prayerTime string,
	notified bool,
	now string,
	updateQuery string,
	rowID int,
) {

	if notified {
		return
	}

	if len(prayerTime) > 8 {
		prayerTime = prayerTime[:8]
	}

	parsedPrayer, err := time.Parse("15:04:05", prayerTime)
	if err != nil {
		log.Println("parse prayer error:", err)
		return
	}

	parsedNow, err := time.Parse("15:04:05", now)
	if err != nil {
		log.Println("parse now error:", err)
		return
	}

	diff := parsedNow.Sub(parsedPrayer)

	if diff < 0 || diff > time.Minute {
		return
	}

	ctx := context.Background()

	rows, err := pool.Query(ctx,
		`SELECT chat_id FROM users
		 WHERE profile_id=$1 AND subscribed=true`,
		profileID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var chatID int64
		if err := rows.Scan(&chatID); err != nil {
			continue
		}

		_, err := bot.Send(&tb.User{ID: chatID},
			fmt.Sprintf("Время намаза: %s", name))
		if err != nil {
			log.Println("Ошибка отправки:", err)
		}
	}

	_, err = pool.Exec(ctx, updateQuery, rowID)
	if err != nil {
		log.Println("Ошибка обновления notified:", err)
	}
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

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatal("Ошибка подключения к базе:", err)
	}
	log.Println("Подключение к базе успешно")

	defer pool.Close()

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

		locationID, err := getOrCreateLocation(ctx, pool, lat, lon)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка сохранения локации")
		}

		method := 3
		school := 1

		profileID, err := getOrCreateProfile(ctx, pool, locationID, method, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка создания профиля")
		}

		_, err = pool.Exec(ctx,
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

		_, err := pool.Exec(context.Background(),
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

		_, err := pool.Exec(context.Background(),
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

		err := pool.QueryRow(context.Background(),
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

		err := pool.QueryRow(ctx,
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

		profileID, err := getOrCreateProfile(ctx, pool, locationID, method, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка обновления мазхаба")
		}

		_, err = pool.Exec(ctx,
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

		err := pool.QueryRow(ctx,
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

		profileID, err := getOrCreateProfile(ctx, pool, locationID, method, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка обновления мазхаба")
		}

		_, err = pool.Exec(ctx,
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

		err := pool.QueryRow(ctx,
			`SELECT p.location_id, p.school
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&locationID, &school)

		if err != nil {
			return nil
		}

		profileID, err := getOrCreateProfile(ctx, pool, locationID, methodID, school)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка обновления метода")
		}

		_, err = pool.Exec(ctx,
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
		ctx := context.Background()

		var profileID int
		var lat float64
		var lon float64
		var method int
		var school int

		err := pool.QueryRow(ctx,
			`SELECT u.profile_id, l.lat, l.lon, p.method, p.school
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 JOIN locations l ON p.location_id = l.id
		 WHERE u.chat_id=$1`,
			chatID,
		).Scan(&profileID, &lat, &lon, &method, &school)

		if err != nil {
			return c.Send("Сначала отправьте геолокацию через /start")
		}

		today := time.Now()

		prayer, found := getPrayerTimes(ctx, pool, profileID, today)

		if !found {

			url := fmt.Sprintf(
				"https://api.aladhan.com/v1/timings?latitude=%f&longitude=%f&method=%d&school=%d",
				lat, lon, method, school,
			)

			resp, err := http.Get(url)
			if err != nil {
				return c.Send("Ошибка получения данных намаза")
			}
			defer resp.Body.Close()

			err = json.NewDecoder(resp.Body).Decode(&prayer)
			if err != nil {
				return c.Send("Ошибка обработки ответа API")
			}

			// сохраняем в базу
			err = savePrayerTimes(ctx, pool, profileID, today, prayer)
			if err != nil {
				log.Println("Ошибка сохранения:", err)
			}
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
			formatTime(prayer.Data.Timings.Fajr),
			formatTime(prayer.Data.Timings.Sunrise),
			formatTime(prayer.Data.Timings.Dhuhr),
			formatTime(prayer.Data.Timings.Asr),
			formatTime(prayer.Data.Timings.Maghrib),
			formatTime(prayer.Data.Timings.Isha),
			lastThird,
		)

		return c.Send(msg)
	})

	go func() {
		for {
			sendPrayerNotifications(bot, pool)
			time.Sleep(30 * time.Second)
		}
	}()

	bot.Start()
}
