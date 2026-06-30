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
	"testing"
)

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
	for i, r := range res.Rows {
		if i >= 10 {
			t.Logf("  …ещё %d строк", len(res.Rows)-i)
			break
		}
		t.Logf("  %v: выручка=%v чеков=%v", r["date"], r["sum_all"], r["count"])
	}
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
