package dooglys

import "testing"

// orders/paycheck хранят дату в display-формате «18 июн. 19:37» под open/close —
// период обязан фильтровать и их (раньше бралось только поле date → данные не за период).
func TestFilterByPeriod_RuDisplayDates(t *testing.T) {
	rows := []Row{
		{"number": "A", "open": "15 июн. 17:31"},
		{"number": "B", "open": "18 июн. 19:37"},
		{"number": "C", "open": "20 июн. 10:00"},
	}
	got := filterByPeriod(rows, "16.06.2026", "19.06.2026")
	if len(got) != 1 || got[0]["number"] != "B" {
		t.Fatalf("ожидалась только строка B (18 июн), got %+v", got)
	}
}

func TestFilterByPeriod_ISODates(t *testing.T) {
	rows := []Row{
		{"date": "2026-06-15"}, {"date": "2026-06-18"}, {"date": "2026-06-20"},
	}
	got := filterByPeriod(rows, "16.06.2026", "19.06.2026")
	if len(got) != 1 || got[0]["date"] != "2026-06-18" {
		t.Fatalf("ISO-фильтр сломан, got %+v", got)
	}
}

// Нет поля даты вовсе (products — агрегат) → строки не режем.
func TestFilterByPeriod_NoDatePassesThrough(t *testing.T) {
	rows := []Row{{"name": "Кофе", "amount": 100.0}, {"name": "Чай", "amount": 50.0}}
	got := filterByPeriod(rows, "16.06.2026", "19.06.2026")
	if len(got) != 2 {
		t.Fatalf("агрегат без дат должен пройти как есть, got %d строк", len(got))
	}
}
