package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"prayerBot/internal/repository"
	"prayerBot/internal/utils"
)

type Service struct {
	repo *repository.Repository
}

func New(r *repository.Repository) *Service {
	return &Service{repo: r}
}

func (s *Service) UpdateMadhab(ctx context.Context, chatID int64, school int) error {

	locationID, method, _, err := s.repo.GetProfile(ctx, chatID)
	if err != nil {
		return err
	}

	profileID, err := s.repo.GetOrCreateProfile(ctx, locationID, method, school)
	if err != nil {
		return err
	}

	return s.repo.UpdateUserProfile(ctx, chatID, profileID)
}

func (s *Service) UpdateMethod(ctx context.Context, chatID int64, method int) error {

	locationID, _, school, err := s.repo.GetProfile(ctx, chatID)
	if err != nil {
		return err
	}

	profileID, err := s.repo.GetOrCreateProfile(ctx, locationID, method, school)
	if err != nil {
		return err
	}

	return s.repo.UpdateUserProfile(ctx, chatID, profileID)
}

func (s *Service) FetchPrayer(lat, lon float64, method, school int) (map[string]string, error) {

	url := fmt.Sprintf(
		"https://api.aladhan.com/v1/timings?latitude=%f&longitude=%f&method=%d&school=%d",
		lat, lon, method, school,
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			Timings map[string]string `json:"timings"`
		} `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&data)
	return data.Data.Timings, err
}

func (s *Service) SaveLocation(ctx context.Context, chatID int64, lat, lon float64) error {

	locationID, err := s.repo.GetOrCreateLocation(ctx, lat, lon)
	if err != nil {
		return err
	}

	method := 3
	school := 0

	profileID, err := s.repo.GetOrCreateProfile(ctx, locationID, method, school)
	if err != nil {
		return err
	}

	return s.repo.SaveUser(ctx, chatID, profileID)
}

func (s *Service) SetSubscription(ctx context.Context, chatID int64, value bool) error {
	return s.repo.UpdateSubscription(ctx, chatID, value)
}

func (s *Service) GetProfileInfo(ctx context.Context, chatID int64) (school int, method int, subscribed bool, err error) {
	return s.repo.GetProfileInfo(ctx, chatID)
}

func (s *Service) GetToday(ctx context.Context, chatID int64) (map[string]string, string, error) {

	profileID, lat, lon, method, school, err := s.repo.GetFullProfile(ctx, chatID)
	if err != nil {
		return nil, "", fmt.Errorf("нет профиля")
	}

	today := time.Now()

	fajr, sunrise, dhuhr, asr, maghrib, isha, found :=
		s.repo.GetPrayerTimes(ctx, profileID, today)

	if !found {

		timings, err := s.FetchPrayer(lat, lon, method, school)
		if err != nil {
			return nil, "", err
		}

		err = s.repo.SavePrayerTimes(ctx, profileID, today,
			timings["Fajr"],
			timings["Sunrise"],
			timings["Dhuhr"],
			timings["Asr"],
			timings["Maghrib"],
			timings["Isha"],
		)

		if err != nil {
			log.Println("Ошибка сохранения:", err)
		}

		fajr = timings["Fajr"]
		sunrise = timings["Sunrise"]
		dhuhr = timings["Dhuhr"]
		asr = timings["Asr"]
		maghrib = timings["Maghrib"]
		isha = timings["Isha"]
	}

	lastThird := utils.LastThirdOfNight(maghrib, fajr)

	result := map[string]string{
		"Fajr":    utils.FormatTime(fajr),
		"Sunrise": utils.FormatTime(sunrise),
		"Dhuhr":   utils.FormatTime(dhuhr),
		"Asr":     utils.FormatTime(asr),
		"Maghrib": utils.FormatTime(maghrib),
		"Isha":    utils.FormatTime(isha),
	}

	return result, lastThird, nil
}

func (s *Service) RunNotifications(send func(chatID int64, text string)) {

	ctx := context.Background()

	now := time.Now()
	today := now.Format("2006-01-02")
	current := now.Format("15:04:05")

	rows, err := s.repo.GetPrayerRows(ctx, today)
	if err != nil {
		log.Println("Ошибка получения prayer rows:", err)
		return
	}

	for _, r := range rows {

		s.check(send, r.ProfileID, "Фаджр", r.Fajr, r.FajrNotified, current,
			`UPDATE prayer_times SET fajr_notified=true WHERE id=$1`, r.ID)

		s.check(send, r.ProfileID, "Зухр", r.Dhuhr, r.DhuhrNotified, current,
			`UPDATE prayer_times SET dhuhr_notified=true WHERE id=$1`, r.ID)

		s.check(send, r.ProfileID, "Аср", r.Asr, r.AsrNotified, current,
			`UPDATE prayer_times SET asr_notified=true WHERE id=$1`, r.ID)

		s.check(send, r.ProfileID, "Магриб", r.Maghrib, r.MaghribNotified, current,
			`UPDATE prayer_times SET maghrib_notified=true WHERE id=$1`, r.ID)

		s.check(send, r.ProfileID, "Иша", r.Isha, r.IshaNotified, current,
			`UPDATE prayer_times SET isha_notified=true WHERE id=$1`, r.ID)
	}
}

func (s *Service) check(
	send func(int64, string),
	profileID int,
	name string,
	prayer string,
	notified bool,
	now string,
	query string,
	rowID int,
) {

	if notified {
		return
	}

	if len(prayer) > 8 {
		prayer = prayer[:8]
	}

	parsedPrayer, err := time.Parse("15:04:05", prayer)
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

	users, err := s.repo.GetSubscribers(ctx, profileID)
	if err != nil {
		log.Println(err)
		return
	}

	for _, id := range users {
		send(id, fmt.Sprintf("Время намаза: %s", name))
	}

	err = s.repo.MarkNotified(ctx, query, rowID)
	if err != nil {
		log.Println("Ошибка обновления notified:", err)
	}
}
