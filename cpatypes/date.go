package cpatypes

import (
	"time"

	"go.recca.ru/sdk/reccatypes"
)

// Date — календарная дата без времени: период выплаты и границы отчёта.
// Определение — в reccatypes: календарь у платформы один.
//
// ⚠ У конверсий границы окна — НАПРОТИВ, полное время (`date-time`), и путать их
// нельзя: у выплаты период — это календарные сутки учёта, а у выборки конверсий —
// момент.
type Date = reccatypes.Date

// NewDate собирает дату из года, месяца и дня в UTC.
func NewDate(year int, month time.Month, day int) Date { return reccatypes.NewDate(year, month, day) }

// DateOf берёт календарную дату момента в его же зоне.
func DateOf(t time.Time) Date { return reccatypes.DateOf(t) }

// ParseDate разбирает дату контрактной формы.
func ParseDate(s string) (Date, error) { return reccatypes.ParseDate(s) }
