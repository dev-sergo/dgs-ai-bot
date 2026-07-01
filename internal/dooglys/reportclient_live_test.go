//go:build live

// Package dooglys — интеграционный тест Report-API против боевого api.dooglys.com
// (только при -tags live). Эквивалент curl из ТЗ:
//
//	GET https://api.dooglys.com/api/v1/reports/report/payment
//	    ?date_from=…&date_to=…&sort_by=date&sort_order=asc
//	access-token: <token>
//	tenant-domain: rukagreka
//
// Запуск (token-режим, внешний):
//
//	DGS_REPORT_BASE='https://api.dooglys.com/api/v1/reports' \
//	DGS_ACCESS_TOKEN='…' DGS_DOMAIN='rukagreka' \
//	go test -tags live ./internal/dooglys/ -run TestLiveReport -v
package dooglys

import (
	"context"
	"os"
	"sort"
	"testing"
)

// dumpColumns печатает имена и пример-значения колонок первой строки — ground truth
// формы боевого Report-API (фикстуры устарели до HTML-формата, доверять им нельзя).
// Это вход для заполнения reportColumnAlias под задачу 3a.
func dumpColumns(t *testing.T, report string, rows []Row) {
	t.Helper()
	if len(rows) == 0 {
		t.Logf("[%s] 0 строк — нечего дампить", report)
		return
	}
	keys := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("[%s] КОЛОНКИ боевого Report-API (%d):", report, len(keys))
	for _, k := range keys {
		t.Logf("    %-22s = %v", k, rows[0][k])
	}
}

func TestLiveReport_Payment(t *testing.T) {
	base := os.Getenv("DGS_REPORT_BASE")
	token := os.Getenv("DGS_ACCESS_TOKEN")
	tenant := os.Getenv("DGS_DOMAIN")
	if base == "" || token == "" || tenant == "" {
		t.Skip("DGS_REPORT_BASE/DGS_ACCESS_TOKEN/DGS_DOMAIN required for live Report-API test")
	}

	cli := NewReportAPIClientToken(base, token, tenant)
	res, err := cli.Fetch(context.Background(), Query{
		Report: "payment",
		From:   "01.06.2025",
		To:     "30.06.2025",
	})
	if err != nil {
		t.Fatalf("Fetch payment: %v", err)
	}
	t.Logf("payment: %d строк (tenant=%s)", len(res.Rows), tenant)
	dumpColumns(t, "payment", res.Rows)
}

// TestLiveReport_ProductsDump — выгрузка формы products для заполнения reportColumnAlias.
func TestLiveReport_ProductsDump(t *testing.T) {
	base := os.Getenv("DGS_REPORT_BASE")
	token := os.Getenv("DGS_ACCESS_TOKEN")
	tenant := os.Getenv("DGS_DOMAIN")
	if base == "" || token == "" || tenant == "" {
		t.Skip("creds required")
	}
	cli := NewReportAPIClientToken(base, token, tenant)
	res, err := cli.Fetch(context.Background(), Query{Report: "products", From: "01.06.2025", To: "30.06.2025"})
	if err != nil {
		t.Fatalf("Fetch products: %v", err)
	}
	t.Logf("products: %d строк", len(res.Rows))
	dumpColumns(t, "products", res.Rows)
}

func TestLiveReport_Personnel(t *testing.T) {
	base := os.Getenv("DGS_REPORT_BASE")
	token := os.Getenv("DGS_ACCESS_TOKEN")
	tenant := os.Getenv("DGS_DOMAIN")
	if base == "" || token == "" || tenant == "" {
		t.Skip("creds required")
	}
	cli := NewReportAPIClientToken(base, token, tenant)
	res, err := cli.Fetch(context.Background(), Query{Report: "personnel", From: "01.06.2025", To: "30.06.2025"})
	if err != nil {
		t.Fatalf("Fetch personnel: %v", err)
	}
	t.Logf("personnel: %d строк", len(res.Rows))
	for i, r := range res.Rows {
		if i >= 10 {
			break
		}
		t.Logf("  %v: выручка=%v чеков=%v", r["name"], r["revenue"], r["total_count"])
	}
}

// TestLiveReport_NewReportsDump — выгрузка формы 4 новых отчётов ТЗ под задачу 4
// (source-order, categories, cash-on-hand, cash-income-outcome). По колонкам заполним
// catalog.Default()/reportFilterColumn точными ключами, как сделали для payment/products.
func TestLiveReport_NewReportsDump(t *testing.T) {
	base := os.Getenv("DGS_REPORT_BASE")
	token := os.Getenv("DGS_ACCESS_TOKEN")
	tenant := os.Getenv("DGS_DOMAIN")
	if base == "" || token == "" || tenant == "" {
		t.Skip("creds required")
	}
	cli := NewReportAPIClientToken(base, token, tenant)
	for _, report := range []string{"source-order", "categories", "cash-on-hand", "cash-income-outcome"} {
		res, err := cli.Fetch(context.Background(), Query{Report: report, From: "01.06.2025", To: "30.06.2025"})
		if err != nil {
			t.Errorf("Fetch %s: %v", report, err)
			continue
		}
		t.Logf("%s: %d строк", report, len(res.Rows))
		dumpColumns(t, report, res.Rows)
	}
}
