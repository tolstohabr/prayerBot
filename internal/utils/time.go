package utils

import (
	"math"
	"time"
)

func RoundCoord(v float64) float64 {
	return math.Round(v*100) / 100
}

func FormatTime(t string) string {
	if len(t) >= 5 {
		return t[:5]
	}
	return t
}

func LastThirdOfNight(maghribStr, fajrStr string) string {
	layout := "15:04"

	maghrib, _ := time.Parse(layout, maghribStr)
	fajr, _ := time.Parse(layout, fajrStr)

	if fajr.Before(maghrib) {
		fajr = fajr.Add(24 * time.Hour)
	}

	night := fajr.Sub(maghrib)
	return fajr.Add(-night / 3).Format(layout)
}
