package dooglys

import (
	"encoding/json"
	"os"
	"testing"
)

// capturedHTML читает сохранённый HTML из директории raw/ (снят харвестером с живого Dooglys).
// Если файл отсутствует — тест пропускается (не блокирует CI без реального трафика).
func capturedHTML(t *testing.T, name string) string {
	t.Helper()
	path := "../../docs/contracts/fixtures/raw/" + name
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("raw HTML %q not found (run harvester first): %v", path, err)
	}
	return string(data)
}

// fixtureRows читает rows из нормализованной JSON-фикстуры.
func fixtureRows(t *testing.T, slug string) []Row {
	t.Helper()
	path := "../../docs/contracts/fixtures/" + slug + ".json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture %q: %v", path, err)
	}
	var f struct {
		Rows []Row `json:"rows"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("parse fixture %q: %v", path, err)
	}
	return f.Rows
}

// TestParseGrid_Payment проверяет, что GridView-парсер корректно извлекает строки
// из сохранённого HTML выручки: правильные ключи, числа как float64, дата как ISO.
func TestParseGrid_Payment(t *testing.T) {
	html := capturedHTML(t, "payment.html")
	q := Query{Report: "payment", From: "01.06.2026", To: "22.06.2026"}

	rows, _, _ := parseGrid(html, q)
	if len(rows) == 0 {
		t.Fatal("parseGrid returned 0 rows for payment")
	}

	// Ключи, которые ОБЯЗАНЫ быть в каждой строке (из каталога).
	required := []string{"date", "kol_vo_chekov", "sum_card", "sum_cash", "onlayn", "sbp", "sum_all", "sredniy_chek"}
	for i, row := range rows {
		for _, key := range required {
			if _, ok := row[key]; !ok {
				t.Errorf("row[%d]: missing required field %q", i, key)
			}
		}
		// date должна быть ISO YYYY-MM-DD
		if date, ok := row["date"].(string); ok {
			if len(date) != 10 || date[4] != '-' || date[7] != '-' {
				t.Errorf("row[%d]: date %q not in ISO format YYYY-MM-DD", i, date)
			}
		} else {
			t.Errorf("row[%d]: date field has wrong type %T", i, row["date"])
		}
		// Числовые поля должны быть float64
		for _, numKey := range []string{"kol_vo_chekov", "sum_all", "sum_card"} {
			if v := row[numKey]; v != nil {
				if _, ok := v.(float64); !ok {
					t.Errorf("row[%d]: %q is %T, want float64", i, numKey, v)
				}
			}
		}
	}
	t.Logf("payment: %d rows, first=%v", len(rows), rows[0])
}

// TestParseGrid_Products проверяет структуру строк отчёта Товары.
func TestParseGrid_Products(t *testing.T) {
	html := capturedHTML(t, "products.html")
	q := Query{Report: "products", From: "01.06.2026", To: "22.06.2026"}

	rows, _, _ := parseGrid(html, q)
	if len(rows) == 0 {
		t.Fatal("parseGrid returned 0 rows for products")
	}

	required := []string{"name", "quantity", "amount", "profit"}
	for i, row := range rows {
		for _, key := range required {
			if _, ok := row[key]; !ok {
				t.Errorf("row[%d]: missing %q", i, key)
			}
		}
	}
	t.Logf("products: %d rows, first name=%v amount=%v", len(rows), rows[0]["name"], rows[0]["amount"])
}

// TestParseGrid_Paycheck проверяет структуру строк Чеков.
func TestParseGrid_Paycheck(t *testing.T) {
	html := capturedHTML(t, "paycheck.html")
	q := Query{Report: "paycheck", From: "01.06.2026", To: "22.06.2026"}

	rows, _, _ := parseGrid(html, q)
	if len(rows) == 0 {
		t.Fatal("parseGrid returned 0 rows for paycheck")
	}

	required := []string{"number", "paid", "tip_oplaty"}
	for i, row := range rows {
		for _, key := range required {
			if _, ok := row[key]; !ok {
				t.Errorf("row[%d]: missing %q", i, key)
			}
		}
	}
	t.Logf("paycheck: %d rows", len(rows))
}

// TestParseGrid_Orders проверяет структуру строк Заказов.
func TestParseGrid_Orders(t *testing.T) {
	html := capturedHTML(t, "orders.html")
	q := Query{Report: "orders", From: "01.06.2026", To: "22.06.2026"}

	rows, _, _ := parseGrid(html, q)
	if len(rows) == 0 {
		t.Fatal("parseGrid returned 0 rows for orders")
	}

	required := []string{"number", "paid", "status"}
	for i, row := range rows {
		for _, key := range required {
			if _, ok := row[key]; !ok {
				t.Errorf("row[%d]: missing %q", i, key)
			}
		}
	}
	t.Logf("orders: %d rows", len(rows))
}

// TestNormalizeCell покрывает все ветки нормализации ячеек.
func TestNormalizeCell(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"", nil},
		{"—", nil},
		{"18.06.2026", "2026-06-18"},
		{"01.01.2025", "2025-01-01"},
		{"510,74", 510.74},
		{"1 027,87", 1027.87},
		{"4", float64(4)},
		{"0,00", float64(0)},
		{"Наличные", "Наличные"},
		{"127,68", 127.68},
	}
	for _, tc := range cases {
		got := normalizeCell(tc.in)
		if got != tc.want {
			t.Errorf("normalizeCell(%q) = %v (%T), want %v (%T)", tc.in, got, got, tc.want, tc.want)
		}
	}
}

// TestBuildURL проверяет, что buildURL корректно формирует URL с фильтрами.
func TestBuildURL(t *testing.T) {
	c := NewHTMLClient("https://example.dooglys.com", "cookie=test")
	q := Query{
		Report: "payment",
		From:   "01.06.2026",
		To:     "22.06.2026",
		Filters: []QueryFilter{
			{Field: "locality", Param: "locality_id", UUIDs: []string{"uuid-1", "uuid-2"}},
			{Field: "sale_point", Param: "sale_point_id", UUIDs: []string{"uuid-3"}},
			{Field: "order_number", Param: "order_number", Names: []string{"RU1"}},
		},
	}
	got := c.buildURL(q)
	t.Logf("URL: %s", got)

	for _, want := range []string{
		"BaseReportForm%5Bperiod%5D=01.06.2026-22.06.2026",
		"BaseReportForm%5Blocality_id%5D%5B%5D=uuid-1",
		"BaseReportForm%5Blocality_id%5D%5B%5D=uuid-2",
		"BaseReportForm%5Bsale_point_id%5D%5B%5D=uuid-3",
		"BaseReportForm%5Border_number%5D=RU1",
	} {
		if !containsStr(got, want) {
			t.Errorf("URL missing %q", want)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSub(s, sub))
}

func findSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
