package dates

import (
	"errors"
	"testing"
	"time"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestResolve(t *testing.T) {
	msk := mustLoad(t, "Europe/Moscow")
	// 2026-06-19 10:00 MSK — пятница.
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, msk)

	cases := []struct {
		token    string
		wantFrom string
		wantTo   string
	}{
		{"today", "19.06.2026", "19.06.2026"},
		{"yesterday", "18.06.2026", "18.06.2026"},
		{"last_7_days", "13.06.2026", "19.06.2026"},
		{"last_14_days", "06.06.2026", "19.06.2026"},
		{"last_30_days", "21.05.2026", "19.06.2026"},
		{"last_90_days", "22.03.2026", "19.06.2026"},
		{"last_3_months", "19.03.2026", "19.06.2026"},
		{"this_week", "15.06.2026", "19.06.2026"}, // понедельник 15-е
		{"last_week", "08.06.2026", "14.06.2026"}, // прошлая неделя пн 8-е — вс 14-е
		{"this_month", "01.06.2026", "19.06.2026"},
		{"last_month", "01.05.2026", "31.05.2026"},
	}
	for _, c := range cases {
		got, err := Resolve(c.token, msk, now)
		if err != nil {
			t.Fatalf("%s: %v", c.token, err)
		}
		if got.From != c.wantFrom || got.To != c.wantTo {
			t.Errorf("%s = %s..%s, want %s..%s", c.token, got.From, got.To, c.wantFrom, c.wantTo)
		}
	}
}

func TestResolveTimezoneBoundary(t *testing.T) {
	// 2026-06-19 23:30 MSK — это уже 20-е в Екатеринбурге (+2ч).
	msk := mustLoad(t, "Europe/Moscow")
	yekb := mustLoad(t, "Asia/Yekaterinburg")
	now := time.Date(2026, 6, 19, 23, 30, 0, 0, msk)

	gotMsk, _ := Resolve("today", msk, now)
	if gotMsk.From != "19.06.2026" {
		t.Errorf("MSK today = %s, want 19.06.2026", gotMsk.From)
	}
	gotYekb, _ := Resolve("today", yekb, now)
	if gotYekb.From != "20.06.2026" {
		t.Errorf("Yekaterinburg today = %s, want 20.06.2026", gotYekb.From)
	}
}

func TestPrevRange(t *testing.T) {
	got, err := PrevRange(Range{"13.06.2026", "19.06.2026"}) // 7 дней
	if err != nil {
		t.Fatal(err)
	}
	if got.From != "06.06.2026" || got.To != "12.06.2026" {
		t.Errorf("prev = %s..%s, want 06.06.2026..12.06.2026", got.From, got.To)
	}
}

func TestResolveUnknown(t *testing.T) {
	_, err := Resolve("someday", time.UTC, time.Now())
	if err == nil {
		t.Error("ожидалась ошибка для неизвестного токена")
	}
	var e *ErrUnknownToken
	if !errors.As(err, &e) {
		t.Errorf("ожидался *ErrUnknownToken, got %T", err)
	}
	if e.Token != "someday" {
		t.Errorf("Token=%q, want someday", e.Token)
	}
}
