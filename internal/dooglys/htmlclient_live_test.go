//go:build live

// Package dooglys — интеграционный тест с реальным Dooglys (только при -tags live).
// Требует: DGS_COOKIE и DGS_BASE в env.
// Запуск: DGS_COOKIE='...' DGS_BASE='https://...' go test -tags live ./internal/dooglys/ -run TestLive -v
package dooglys

import (
	"context"
	"os"
	"testing"
)

func TestLive_Payment(t *testing.T) {
	cookie := os.Getenv("DGS_COOKIE")
	base := os.Getenv("DGS_BASE")
	if cookie == "" || base == "" {
		t.Skip("DGS_COOKIE and DGS_BASE required for live test")
	}

	c := NewHTMLClient(base, cookie)
	res, err := c.Fetch(context.Background(), Query{
		Report: "payment",
		From:   "01.06.2026",
		To:     "22.06.2026",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res.Rows) == 0 {
		t.Fatal("expected rows, got 0")
	}
	t.Logf("payment: %d rows", len(res.Rows))
	t.Logf("first row: %v", res.Rows[0])

	// Базовые проверки
	for i, row := range res.Rows {
		if _, ok := row["date"]; !ok {
			t.Errorf("row[%d]: missing date", i)
		}
		if _, ok := row["sum_all"].(float64); !ok {
			t.Errorf("row[%d]: sum_all is not float64", i)
		}
	}
}

func TestLive_Products(t *testing.T) {
	cookie := os.Getenv("DGS_COOKIE")
	base := os.Getenv("DGS_BASE")
	if cookie == "" || base == "" {
		t.Skip("DGS_COOKIE and DGS_BASE required for live test")
	}

	c := NewHTMLClient(base, cookie)
	res, err := c.Fetch(context.Background(), Query{
		Report: "products",
		From:   "01.06.2026",
		To:     "22.06.2026",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	t.Logf("products: %d rows", len(res.Rows))
	if len(res.Rows) > 0 {
		t.Logf("first: %v", res.Rows[0])
	}
}
