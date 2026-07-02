package render

import (
	"strings"
	"testing"

	"dgsbot/internal/envelope"
)

// TestGroupThousands — RU-формат: пробел-разделитель тысяч, запятая-десятичные, минус.
func TestGroupThousands(t *testing.T) {
	cases := []struct {
		v        float64
		decimals int
		want     string
	}{
		{0, 2, "0,00"},
		{1234.5, 2, "1 234,50"},
		{1234567, 0, "1 234 567"},
		{-1500.25, 2, "-1 500,25"},
		{999, 0, "999"},
		{1000, 0, "1 000"},
	}
	for _, tc := range cases {
		if got := groupThousands(tc.v, tc.decimals); got != tc.want {
			t.Errorf("groupThousands(%v,%d) = %q, want %q", tc.v, tc.decimals, got, tc.want)
		}
	}
}

// TestFormatNumberByUnit — единица определяет суффикс (₽ / % / без).
func TestFormatNumberByUnit(t *testing.T) {
	cases := []struct {
		v    float64
		unit string
		want string
	}{
		{1500, "RUB", "1 500,00 ₽"},
		{12.5, "percent", "12,50 %"},
		{42, "count", "42"},
		{3.14, "", "3,14"},
	}
	for _, tc := range cases {
		if got := Number(tc.v, tc.unit, "RUB"); got != tc.want {
			t.Errorf("Number(%v,%q) = %q, want %q", tc.v, tc.unit, got, tc.want)
		}
	}
}

// TestMoneyNonRUBCurrency — неизвестная валюта печатается кодом, а не падает.
func TestMoneyNonRUBCurrency(t *testing.T) {
	if got := Money(100, "USD"); got != "100,00 USD" {
		t.Errorf("Money USD = %q", got)
	}
	if got := Money(100, "RUB"); got != "100,00 ₽" {
		t.Errorf("Money RUB = %q", got)
	}
}

// TestReportTitleKnownTypes — все каталожные отчёты (включая personnel) имеют
// человеческий заголовок; суффиксы _compare/_contribution срезаются.
func TestReportTitleKnownTypes(t *testing.T) {
	cases := map[string]string{
		"payment":               "Выручка",
		"products":              "Товары",
		"paycheck":              "Чеки",
		"personnel":             "Персонал",
		"orders":                "Заказы",
		"forecast":              "Прогноз выручки",
		"payment_compare":       "Выручка",
		"payment_contribution":  "Выручка",
		"products_contribution": "Товары",
	}
	for typ, want := range cases {
		if got := Title(typ); got != want {
			t.Errorf("Title(%q) = %q, want %q", typ, got, want)
		}
	}
}

// TestTextEmptyRows — без строк рендер даёт честный «данных нет», а не пустую таблицу.
func TestTextEmptyRows(t *testing.T) {
	e := envelope.Envelope{
		Type:   "payment",
		Period: envelope.Period{From: "01.06.2026", To: "07.06.2026", TZ: "Europe/Moscow"},
	}
	out := Text(e)
	if !strings.Contains(out, "Выручка") {
		t.Errorf("title missing: %q", out)
	}
	if !strings.Contains(out, "данных нет") {
		t.Errorf("empty-data line missing: %q", out)
	}
}

// TestTextTableWithTotals — таблица с колонками/строками и блоком «Итого».
func TestTextTableWithTotals(t *testing.T) {
	e := envelope.Envelope{
		Type:     "payment",
		Currency: "RUB",
		Period:   envelope.Period{From: "01.06.2026", To: "07.06.2026", TZ: "Europe/Moscow"},
		Columns: []envelope.Column{
			{Key: "date", Label: "Дата", Unit: "date"},
			{Key: "sum_all", Label: "Выручка", Unit: "RUB"},
		},
		Rows: []map[string]any{
			{"date": "01.06.2026", "sum_all": float64(1000)},
			{"date": "02.06.2026", "sum_all": float64(2500)},
		},
		Summary: map[string]float64{"sum_all": 3500},
	}
	out := Text(e)
	for _, want := range []string{"Дата", "Выручка", "1 000,00 ₽", "2 500,00 ₽", "Итого:", "3 500,00 ₽"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
