package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	tb "gopkg.in/telebot.v3"
	"prayerBot/internal/service"
	"prayerBot/internal/utils"
)

var methods = map[string]int{
	"Muslim World League (Мир)": 3,
	"Umm Al-Qura (Аравия)":      4,
	"Egyptian (Африка)":         5,
	"Karachi (Азия)":            1,
	"Diyanet (Турция, Европа)":  13,
	"ISNA (Америка)":            2,
}

type Bot struct {
	bot     *tb.Bot
	service *service.Service
}

func New(token string, s *service.Service) *Bot {
	b, err := tb.NewBot(tb.Settings{
		Token:  token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		panic(err)
	}

	bt := &Bot{bot: b, service: s}
	bt.register()
	return bt
}

func (b *Bot) Start() {

	go func() {
		for {
			b.service.RunNotifications(func(id int64, text string) {
				_, err := b.bot.Send(&tb.User{ID: id}, text)
				if err != nil {
					log.Println("Ошибка отправки:", err)
				}
			})
			time.Sleep(30 * time.Second)
		}
	}()

	b.bot.Start()
}

func (b *Bot) register() {

	b.bot.Handle("/start", func(c tb.Context) error {

		btn := tb.ReplyButton{
			Text:     "Отправить геолокацию",
			Location: true,
		}

		markup := &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btn},
			},
		}

		return c.Send("Отправь геолокацию:", markup)
	})

	b.bot.Handle("/settings", func(c tb.Context) error {

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

	b.bot.Handle("Мазхаб", func(c tb.Context) error {

		btnH := tb.ReplyButton{Text: "Ханафи"}
		btnS := tb.ReplyButton{Text: "Шафии"}

		return c.Send("Выберите мазхаб:", &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btnH, btnS},
			},
		})
	})

	b.bot.Handle("Ханафи", func(c tb.Context) error {

		err := b.service.UpdateMadhab(context.Background(), c.Sender().ID, 1)
		if err != nil {
			return c.Send("Ошибка обновления мазхаба")
		}

		return c.Send("Выбран ханафитский мазхаб", &tb.ReplyMarkup{RemoveKeyboard: true})
	})

	b.bot.Handle("Шафии", func(c tb.Context) error {

		err := b.service.UpdateMadhab(context.Background(), c.Sender().ID, 0)
		if err != nil {
			return c.Send("Ошибка обновления мазхаба")
		}

		return c.Send("Выбран шафиитский мазхаб", &tb.ReplyMarkup{RemoveKeyboard: true})
	})

	b.bot.Handle("Метод расчёта", func(c tb.Context) error {

		btns := [][]tb.ReplyButton{
			{
				{Text: "Muslim World League (Мир)"},
				{Text: "Umm Al-Qura (Аравия)"},
			},
			{
				{Text: "Egyptian (Африка)"},
				{Text: "Karachi (Азия)"},
			},
			{
				{Text: "Diyanet (Турция, Европа)"},
				{Text: "ISNA (Америка)"},
			},
		}

		return c.Send("Выберите организацию расчёта:", &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard:  btns,
		})
	})

	b.bot.Handle(tb.OnText, func(c tb.Context) error {

		methodID, ok := methods[c.Text()]
		if !ok {
			return nil
		}

		err := b.service.UpdateMethod(context.Background(), c.Sender().ID, methodID)
		if err != nil {
			return c.Send("Ошибка обновления метода")
		}

		return c.Send("Метод расчёта обновлён", &tb.ReplyMarkup{RemoveKeyboard: true})
	})

	b.bot.Handle("Геолокация", func(c tb.Context) error {

		btn := tb.ReplyButton{
			Text:     "Отправить геолокацию",
			Location: true,
		}

		return c.Send("Отправь новую геолокацию:", &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard:  [][]tb.ReplyButton{{btn}},
		})
	})

	b.bot.Handle(tb.OnLocation, func(c tb.Context) error {

		loc := c.Message().Location
		if loc == nil {
			return nil
		}

		lat := utils.RoundCoord(float64(loc.Lat))
		lon := utils.RoundCoord(float64(loc.Lng))
		chatID := c.Sender().ID

		err := b.service.SaveLocation(context.Background(), chatID, lat, lon)
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка сохранения локации")
		}

		msg := fmt.Sprintf(
			"Геолокация сохранена\n\nШирота: %.2f\nДолгота: %.2f",
			lat, lon,
		)

		return c.Send(msg, &tb.ReplyMarkup{RemoveKeyboard: true})
	})

	b.bot.Handle("Подписка", func(c tb.Context) error {

		btnOn := tb.ReplyButton{Text: "Подписаться"}
		btnOff := tb.ReplyButton{Text: "Отписаться"}

		return c.Send("Управление уведомлениями", &tb.ReplyMarkup{
			ResizeKeyboard: true,
			ReplyKeyboard: [][]tb.ReplyButton{
				{btnOn, btnOff},
			},
		})
	})

	b.bot.Handle("Подписаться", func(c tb.Context) error {

		err := b.service.SetSubscription(context.Background(), c.Sender().ID, true)
		if err != nil {
			return c.Send("Ошибка подписки")
		}

		return c.Send("Вы подписались на уведомления о намазе")
	})

	b.bot.Handle("Отписаться", func(c tb.Context) error {

		err := b.service.SetSubscription(context.Background(), c.Sender().ID, false)
		if err != nil {
			return c.Send("Ошибка отписки")
		}

		return c.Send("Вы отписались от уведомлений")
	})

	b.bot.Handle("Профиль", func(c tb.Context) error {

		school, method, sub, err :=
			b.service.GetProfileInfo(context.Background(), c.Sender().ID)

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

		subText := "Нет"
		if sub {
			subText = "Да"
		}

		msg := fmt.Sprintf(
			"Профиль\n\nМазхаб: %s\nМетод: %s\nПодписка: %s",
			madhab, methodName, subText,
		)

		return c.Send(msg)
	})

	b.bot.Handle("/today", func(c tb.Context) error {

		data, last, err :=
			b.service.GetToday(context.Background(), c.Sender().ID)

		if err != nil {
			return c.Send("Сначала отправьте геолокацию через /start")
		}

		msg := fmt.Sprintf(
			"Расписание на сегодня:\n\n"+
				"Фаджр: %s\n"+
				"Восход: %s\n"+
				"Зухр: %s\n"+
				"Аср: %s\n"+
				"Магриб: %s\n"+
				"Иша: %s\n\n"+
				"Последняя треть ночи: %s",
			data["Fajr"],
			data["Sunrise"],
			data["Dhuhr"],
			data["Asr"],
			data["Maghrib"],
			data["Isha"],
			last,
		)

		return c.Send(msg)
	})
}
