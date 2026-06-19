// Package dates — резолв относительных периодов в абсолютные даты по таймзоне тенанта.
// LLM выдаёт токен (today/last_7_days/...), даты считает Go — не модель.
package dates

import (
	"fmt"
	"time"
)

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
	case "last_30_days":
		return Range{df(today.AddDate(0, 0, -29)), df(today)}, nil
	case "this_week": // неделя с понедельника
		offset := (int(today.Weekday()) + 6) % 7
		mon := today.AddDate(0, 0, -offset)
		return Range{df(mon), df(today)}, nil
	case "this_month":
		first := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
		return Range{df(first), df(today)}, nil
	case "last_month":
		firstThis := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
		lastPrev := firstThis.AddDate(0, 0, -1)
		firstPrev := time.Date(lastPrev.Year(), lastPrev.Month(), 1, 0, 0, 0, 0, loc)
		return Range{df(firstPrev), df(lastPrev)}, nil
	default:
		return Range{}, fmt.Errorf("неизвестный токен периода: %q", token)
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
