//go:build live

// Package dooglys — интеграционный тест APIClient против реального Dooglys JSON API
// (только при -tags live). Требует DGS_BASE, DGS_DOMAIN, DGS_LOGIN, DGS_PASSWORD в env.
// Запуск:
//   DGS_BASE='https://google.dooglys.com' DGS_DOMAIN='google' \
//   DGS_LOGIN='...' DGS_PASSWORD='...' go test -tags live ./internal/dooglys/ -run TestLiveAPI -v
package dooglys

import (
	"context"
	"os"
	"testing"
)

func TestLiveAPI_Payment(t *testing.T) {
	base := os.Getenv("DGS_BASE")
	domain := os.Getenv("DGS_DOMAIN")
	login := os.Getenv("DGS_LOGIN")
	pass := os.Getenv("DGS_PASSWORD")
	if base == "" || domain == "" || login == "" || pass == "" {
		t.Skip("DGS_BASE/DGS_DOMAIN/DGS_LOGIN/DGS_PASSWORD required for live API test")
	}

	c := NewAPIClient(base, domain, login, pass)
	// Окно мая 2025 — где лежат боевые заказы тестового тенанта.
	res, err := c.Fetch(context.Background(), Query{
		Report: "payment",
		From:   "01.05.2025",
		To:     "31.05.2025",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	t.Logf("payment: %d дней", len(res.Rows))

	var totalChecks, totalRevenue, totalReturns float64
	for i, row := range res.Rows {
		if _, ok := row["date"].(string); !ok {
			t.Errorf("row[%d]: date не string: %v", i, row["date"])
		}
		if _, ok := row["sum_all"].(float64); !ok {
			t.Errorf("row[%d]: sum_all не float64: %v", i, row["sum_all"])
		}
		totalChecks += asF(row["kol_vo_chekov"])
		totalRevenue += asF(row["sum_all"])
		totalReturns += asF(row["return_sum"])
		t.Logf("  %v: чеков=%v выручка=%v нал=%v карта=%v возвр=%v",
			row["date"], row["kol_vo_chekov"], row["sum_all"],
			row["sum_cash"], row["sum_card"], row["return_sum"])
	}
	t.Logf("ИТОГО: чеков=%.0f выручка(net)=%.2f возвраты=%.2f", totalChecks, totalRevenue, totalReturns)
}

func asF(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func TestLiveAPI_Products(t *testing.T) {
	base, domain := os.Getenv("DGS_BASE"), os.Getenv("DGS_DOMAIN")
	login, pass := os.Getenv("DGS_LOGIN"), os.Getenv("DGS_PASSWORD")
	if base == "" || domain == "" || login == "" || pass == "" {
		t.Skip("creds required")
	}
	c := NewAPIClient(base, domain, login, pass)

	// Полный товарный отчёт за май 2025.
	res, err := c.Fetch(context.Background(), Query{Report: "products", From: "01.05.2025", To: "31.05.2025"})
	if err != nil {
		t.Fatalf("Fetch products: %v", err)
	}
	t.Logf("товаров: %d", len(res.Rows))
	for i, r := range res.Rows {
		if i >= 5 {
			break
		}
		t.Logf("  %v: кол-во=%v выручка=%v", r["name"], r["quantity"], r["amount"])
	}

	// Drill-down по имени товара.
	f := []QueryFilter{{Field: "product", Param: "product_id", Names: []string{"Бизнес ланч"}}}
	one, err := c.Fetch(context.Background(), Query{Report: "products", From: "01.05.2025", To: "31.05.2025", Filters: f})
	if err != nil {
		t.Fatalf("Fetch products filtered: %v", err)
	}
	t.Logf("drill-down «Бизнес ланч»: %d строк → %+v", len(one.Rows), one.Rows)

	// Индекс товаров для резолвера.
	idx, err := c.ProductIndex(context.Background())
	if err != nil {
		t.Fatalf("ProductIndex: %v", err)
	}
	t.Logf("ProductIndex: %d уникальных товаров", len(idx))
}
