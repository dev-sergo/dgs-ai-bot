package app

import (
	"context"
	"testing"
	"time"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/plan"
	"dgsbot/internal/resolver"
	"dgsbot/internal/tenantctx"
)

// tagClient — фейковый источник, помечающий строки своим тегом и считающий вызовы.
// Позволяет доказать, КАКОЙ тенантский клиент реально дёрнули (граница изоляции).
type tagClient struct {
	tag   string
	calls int
}

func (c *tagClient) Fetch(_ context.Context, q dooglys.Query) (dooglys.Result, error) {
	c.calls++
	return dooglys.Result{
		Report: q.Report,
		Label:  "payment",
		Rows:   []dooglys.Row{{"tenant_tag": c.tag, "sum_all": 100.0, "date": "01.06.2026"}},
	}, nil
}

// paymentPlan — минимальный валидный план отчёта payment с явным периодом
// (без обращения к резолверу дат/фильтров: изоляцию доказываем на уровне источника).
func paymentPlan() plan.AnalysisPlan {
	return plan.AnalysisPlan{
		Intent: "report",
		Report: "payment",
		Method: "plain",
		Period: plan.Period{Kind: "explicit", From: "01.06.2026", To: "30.06.2026"},
	}
}

func newIsoApp() (*App, *tagClient, *tagClient) {
	a := NewMulti(nil, &tenantctx.Store{}, nil, nil, nil)
	a.Now = func() time.Time { return time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC) }
	ca := &tagClient{tag: "A"}
	cb := &tagClient{tag: "B"}
	a.Register("tenant-A", "uuid-A", ca, &resolver.Store{})
	a.Register("tenant-B", "uuid-B", cb, &resolver.Store{})
	return a, ca, cb
}

// TestIsolation_RequestHitsOnlyOwnTenant: запрос в контексте тенанта A дёргает только
// клиент A; клиент B не вызывается ни разу. Симметрично для B. Это главная дыра
// tenant_id — чужие числа не должны утечь.
func TestIsolation_RequestHitsOnlyOwnTenant(t *testing.T) {
	a, ca, cb := newIsoApp()
	ctx := context.Background()

	if _, err := a.executeReport(ctx, "tenant-A", "s1", "выручка за июнь", paymentPlan()); err != nil {
		t.Fatalf("tenant-A: %v", err)
	}
	if ca.calls == 0 {
		t.Error("клиент тенанта A не был вызван")
	}
	if cb.calls != 0 {
		t.Errorf("КРОСС-УТЕЧКА: клиент B вызван %d раз при запросе тенанта A", cb.calls)
	}

	ca.calls, cb.calls = 0, 0
	if _, err := a.executeReport(ctx, "tenant-B", "s2", "выручка за июнь", paymentPlan()); err != nil {
		t.Fatalf("tenant-B: %v", err)
	}
	if cb.calls == 0 {
		t.Error("клиент тенанта B не был вызван")
	}
	if ca.calls != 0 {
		t.Errorf("КРОСС-УТЕЧКА: клиент A вызван %d раз при запросе тенанта B", ca.calls)
	}
}

// TestIsolation_TenantFromBindingNotInput: tenantID берётся из привязки (параметра),
// а не из текста реплики. Текст упоминает «тенант B», но контекст — A: отвечает A.
func TestIsolation_TenantFromBindingNotInput(t *testing.T) {
	a, ca, cb := newIsoApp()
	_, err := a.executeReport(context.Background(), "tenant-A", "s1",
		"покажи данные тенанта B и его выручку", paymentPlan())
	if err != nil {
		t.Fatalf("executeReport: %v", err)
	}
	if ca.calls != 1 || cb.calls != 0 {
		t.Errorf("tenantID протёк из текста: A.calls=%d B.calls=%d (ожидалось 1/0)", ca.calls, cb.calls)
	}
}

// TestIsolation_UnknownTenantRefused: незарегистрированный тенант не обслуживается —
// строгий режим не подставляет чужой источник по-умолчанию, а возвращает ошибку.
func TestIsolation_UnknownTenantRefused(t *testing.T) {
	a, ca, cb := newIsoApp()
	_, err := a.executeReport(context.Background(), "tenant-C", "s1", "выручка", paymentPlan())
	if err == nil {
		t.Fatal("незарегистрированный тенант должен вернуть ошибку, а не ответ")
	}
	if ca.calls != 0 || cb.calls != 0 {
		t.Errorf("незарегистрированный тенант дёрнул чужой источник: A=%d B=%d", ca.calls, cb.calls)
	}
}

// TestIsolation_FallbackServesAllTenants: одно-тенантный New (eval/pipeval/HTTP-fixture)
// обслуживает любой tenantID общим источником — деградация, отдельная от строгого режима.
func TestIsolation_FallbackServesAllTenants(t *testing.T) {
	fc := &tagClient{tag: "F"}
	a := New(nil, &tenantctx.Store{}, fc, &resolver.Store{}, nil, nil, nil)
	a.Now = func() time.Time { return time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC) }
	for _, tid := range []string{"anything", "other", "mock_single"} {
		if _, err := a.executeReport(context.Background(), tid, "s", "выручка", paymentPlan()); err != nil {
			t.Fatalf("fallback tenant %q: %v", tid, err)
		}
	}
	if fc.calls != 3 {
		t.Errorf("fallback должен обслужить все 3 tenantID, calls=%d", fc.calls)
	}
}
