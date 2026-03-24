package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDB(dbURL string) *pgxpool.Pool {
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		panic(err)
	}
	return pool
}

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetOrCreateLocation(ctx context.Context, lat, lon float64) (int, error) {
	var id int

	err := r.db.QueryRow(ctx,
		`INSERT INTO locations (lat, lon)
		 VALUES ($1, $2)
		 ON CONFLICT (lat, lon)
		 DO UPDATE SET lat = EXCLUDED.lat
		 RETURNING id`,
		lat, lon,
	).Scan(&id)

	return id, err
}

func (r *Repository) GetOrCreateProfile(ctx context.Context, locationID, method, school int) (int, error) {
	var id int

	err := r.db.QueryRow(ctx,
		`INSERT INTO prayer_profiles (location_id, method, school)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (location_id,method,school)
		 DO UPDATE SET method = EXCLUDED.method
		 RETURNING id`,
		locationID, method, school,
	).Scan(&id)

	return id, err
}

func (r *Repository) SaveUser(ctx context.Context, chatID int64, profileID int) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (chat_id, profile_id)
		 VALUES ($1,$2)
		 ON CONFLICT (chat_id)
		 DO UPDATE SET profile_id = EXCLUDED.profile_id`,
		chatID, profileID,
	)
	return err
}

func (r *Repository) UpdateUserProfile(ctx context.Context, chatID int64, profileID int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET profile_id=$1 WHERE chat_id=$2`,
		profileID, chatID,
	)
	return err
}

func (r *Repository) UpdateSubscription(ctx context.Context, chatID int64, value bool) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET subscribed=$1 WHERE chat_id=$2`,
		value, chatID,
	)
	return err
}

func (r *Repository) GetProfile(ctx context.Context, chatID int64) (locationID, method, school int, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT p.location_id, p.method, p.school
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 WHERE u.chat_id=$1`,
		chatID,
	).Scan(&locationID, &method, &school)

	return
}

func (r *Repository) GetFullProfile(ctx context.Context, chatID int64) (profileID int, lat, lon float64, method, school int, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT u.profile_id, l.lat, l.lon, p.method, p.school
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 JOIN locations l ON p.location_id = l.id
		 WHERE u.chat_id=$1`,
		chatID,
	).Scan(&profileID, &lat, &lon, &method, &school)

	return
}

func (r *Repository) GetProfileInfo(ctx context.Context, chatID int64) (school int, method int, subscribed bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT p.school, p.method, u.subscribed
		 FROM users u
		 JOIN prayer_profiles p ON u.profile_id = p.id
		 WHERE u.chat_id=$1`,
		chatID,
	).Scan(&school, &method, &subscribed)

	return
}

func (r *Repository) SavePrayerTimes(
	ctx context.Context,
	profileID int,
	date time.Time,
	fajr, sunrise, dhuhr, asr, maghrib, isha string,
) error {

	_, err := r.db.Exec(ctx,
		`INSERT INTO prayer_times
		(profile_id, date, fajr, sunrise, dhuhr, asr, maghrib, isha)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (profile_id, date) DO NOTHING`,
		profileID,
		date.Format("2006-01-02"),
		fajr, sunrise, dhuhr, asr, maghrib, isha,
	)

	return err
}

func (r *Repository) GetPrayerTimes(
	ctx context.Context,
	profileID int,
	date time.Time,
) (fajr, sunrise, dhuhr, asr, maghrib, isha string, found bool) {

	err := r.db.QueryRow(ctx,
		`SELECT fajr, sunrise, dhuhr, asr, maghrib, isha
		 FROM prayer_times
		 WHERE profile_id=$1 AND date=$2`,
		profileID,
		date.Format("2006-01-02"),
	).Scan(&fajr, &sunrise, &dhuhr, &asr, &maghrib, &isha)

	if err != nil {
		return "", "", "", "", "", "", false
	}

	return fajr, sunrise, dhuhr, asr, maghrib, isha, true
}

type PrayerRow struct {
	ID        int
	ProfileID int

	Fajr    string
	Dhuhr   string
	Asr     string
	Maghrib string
	Isha    string

	FajrNotified    bool
	DhuhrNotified   bool
	AsrNotified     bool
	MaghribNotified bool
	IshaNotified    bool
}

func (r *Repository) GetPrayerRows(ctx context.Context, date string) ([]PrayerRow, error) {

	rows, err := r.db.Query(ctx,
		`SELECT id, profile_id,
		        fajr, dhuhr, asr, maghrib, isha,
		        fajr_notified, dhuhr_notified, asr_notified,
		        maghrib_notified, isha_notified
		 FROM prayer_times
		 WHERE date=$1`,
		date,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PrayerRow

	for rows.Next() {
		var rrow PrayerRow

		err := rows.Scan(
			&rrow.ID,
			&rrow.ProfileID,
			&rrow.Fajr,
			&rrow.Dhuhr,
			&rrow.Asr,
			&rrow.Maghrib,
			&rrow.Isha,
			&rrow.FajrNotified,
			&rrow.DhuhrNotified,
			&rrow.AsrNotified,
			&rrow.MaghribNotified,
			&rrow.IshaNotified,
		)

		if err == nil {
			result = append(result, rrow)
		}
	}

	return result, nil
}

func (r *Repository) GetSubscribers(ctx context.Context, profileID int) ([]int64, error) {

	rows, err := r.db.Query(ctx,
		`SELECT chat_id FROM users
		 WHERE profile_id=$1 AND subscribed=true`,
		profileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []int64

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			result = append(result, id)
		}
	}

	return result, nil
}

func (r *Repository) MarkNotified(ctx context.Context, query string, id int) error {
	_, err := r.db.Exec(ctx, query, id)
	return err
}
