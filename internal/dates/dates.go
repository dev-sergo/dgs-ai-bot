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
	case "this_month_full": // полный текущий месяц [1-е .. последний день]; для прогноза
		first := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
		last := first.AddDate(0, 1, -1)
		return Range{df(first), df(last)}, nil
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

// NormalizeExplicitYear чинит год в явных датах, которые проставила модель: на запрос
// без года («июнь», «с 1 по 15 июня») LLM не знает «сейчас» и пинит прошлый год (типично
// 2023) → выборка пустая, «данных нет». Год месяца/диапазона переносим к актуальному.
//
// hasYearInText=true (пользователь сам назвал год — «июнь 2024», «01.06.2025») → НЕ трогаем:
// это осознанный выбор периода. Если перенос в текущий год даёт период целиком в будущем
// (сегодня раньше его начала) — берём прошлый год: ближайшее СЛУЧИВШЕЕСЯ вхождение месяца.
func NormalizeExplicitYear(from, to string, hasYearInText bool, loc *time.Location, now time.Time) (string, string) {
	if hasYearInText {
		return from, to
	}
	f, errF := time.ParseInLocation(layout, from, loc)
	t, errT := time.ParseInLocation(layout, to, loc)
	if errF != nil || errT != nil {
		return from, to // не разобрали — оставляем как есть (валидатор/выборка разберётся)
	}
	cy := now.In(loc).Year()
	if f.Year() == cy && t.Year() == cy {
		return from, to // уже текущий год
	}
	nf := time.Date(cy, f.Month(), f.Day(), 0, 0, 0, 0, loc)
	nt := time.Date(cy, t.Month(), t.Day(), 0, 0, 0, 0, loc)
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if nf.After(today) { // период ещё не наступил в этом году → прошлый год
		nf = nf.AddDate(-1, 0, 0)
		nt = nt.AddDate(-1, 0, 0)
	}
	return df(nf), df(nt)
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
