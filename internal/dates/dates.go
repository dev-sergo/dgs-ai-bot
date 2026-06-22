// Package dates — резолв относительных периодов в абсолютные даты по таймзоне тенанта.
// LLM выдаёт токен (today/last_7_days/...), даты считает Go — не модель.
package dates

import (
	"fmt"
	"time"
)

// ErrUnknownToken возвращается когда токен периода не входит в white-list.
// Типизированная ошибка позволяет оркестратору вернуть clarify вместо 500.
type ErrUnknownToken struct{ Token string }

func (e *ErrUnknownToken) Error() string {
	return fmt.Sprintf("неизвестный токен периода: %q", e.Token)
}

// Range — абсолютный период (включительно), в формате DD.MM.YYYY для Dooglys.
type Range struct {
	From string
	To   string
}

const layout = "02.01.2006"

// Resolve переводит относительный токен в абсолютный период по локали и «сейчас».
// now передаётся явно — для детерминированных тестов.
func Resolve(token string, loc *time.Location, now time.Time) (Range, error) {
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)

	switch token {
	case "today":
		return day(today), nil
	case "yesterday":
		return day(today.AddDate(0, 0, -1)), nil
	case "last_7_days":
		return Range{df(today.AddDate(0, 0, -6)), df(today)}, nil
	case "last_14_days":
		return Range{df(today.AddDate(0, 0, -13)), df(today)}, nil
	case "last_30_days":
		return Range{df(today.AddDate(0, 0, -29)), df(today)}, nil
	case "this_week": // неделя с понедельника
		offset := (int(today.Weekday()) + 6) % 7
		mon := today.AddDate(0, 0, -offset)
		return Range{df(mon), df(today)}, nil
	case "last_week": // прошлая календарная неделя (пн–вс)
		offset := (int(today.Weekday()) + 6) % 7
		lastSun := today.AddDate(0, 0, -offset-1)
		lastMon := lastSun.AddDate(0, 0, -6)
		return Range{df(lastMon), df(lastSun)}, nil
	case "this_month":
		first := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
		return Range{df(first), df(today)}, nil
	case "last_month":
		firstThis := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
		lastPrev := firstThis.AddDate(0, 0, -1)
		firstPrev := time.Date(lastPrev.Year(), lastPrev.Month(), 1, 0, 0, 0, 0, loc)
		return Range{df(firstPrev), df(lastPrev)}, nil
	case "last_90_days":
		return Range{df(today.AddDate(0, 0, -89)), df(today)}, nil
	case "last_3_months":
		return Range{df(today.AddDate(0, -3, 0)), df(today)}, nil
	default:
		return Range{}, &ErrUnknownToken{Token: token}
	}
}

// PrevRange возвращает предыдущий равный по длине период (для compare).
func PrevRange(r Range) (Range, error) {
	from, err := time.Parse(layout, r.From)
	if err != nil {
		return Range{}, err
	}
	to, err := time.Parse(layout, r.To)
	if err != nil {
		return Range{}, err
	}
	days := int(to.Sub(from).Hours()/24) + 1
	prevTo := from.AddDate(0, 0, -1)
	prevFrom := prevTo.AddDate(0, 0, -(days - 1))
	return Range{prevFrom.Format(layout), prevTo.Format(layout)}, nil
}

func day(t time.Time) Range { return Range{df(t), df(t)} }
func df(t time.Time) string { return t.Format(layout) }
