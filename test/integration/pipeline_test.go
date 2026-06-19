// Package integration — сквозные тесты пайплайна без GPU: StubPlanner + FixtureClient.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"dgsbot/internal/app"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/planner"
	"dgsbot/internal/tenantctx"
)

const fixturesDir = "../../docs/contracts/fixtures"

func newApp(t *testing.T) *app.App {
	t.Helper()
	tenants, err := tenantctx.Load(fixturesDir + "/tenants.example.json")
	if err != nil {
		t.Fatalf("load tenants: %v", err)
	}
	a := app.New(planner.NewStub(), tenants, dooglys.NewFixtureClient(fixturesDir))
	// Фиксированное «сейчас» для детерминизма дат: 2026-06-19 10:00 UTC.
	a.Now = func() time.Time { return time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC) }
	return a
}

func TestRevenueLastWeek(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "покажи выручку за последнюю неделю")
	if err != nil {
		t.Fatal(err)
	}
	if !ans.Validation.OK || ans.Envelope == nil {
		t.Fatalf("ожидался валидный ответ с envelope: %+v", ans.Validation)
	}
	if ans.Plan.Report != "payment" {
		t.Fatalf("report = %s, want payment", ans.Plan.Report)
	}
	// last_7_days от 19.06 → 13.06..19.06; в фикстуре попадают 15,16,18 июня.
	// sum_all: 416.00 + 115.00 + 510.74 = 1041.74
	if got := ans.Envelope.Summary["sum_all"]; got != 1041.74 {
		t.Errorf("сумма выручки = %v, want 1041.74", got)
	}
	if ans.Envelope.Period.From != "13.06.2026" || ans.Envelope.Period.To != "19.06.2026" {
		t.Errorf("период = %s..%s, want 13.06.2026..19.06.2026", ans.Envelope.Period.From, ans.Envelope.Period.To)
	}
	if !strings.Contains(ans.Text, "Выручка") {
		t.Errorf("в тексте нет заголовка 'Выручка':\n%s", ans.Text)
	}
}

func TestProductsThisMonth(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "какие товары продавались за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Plan.Report != "products" {
		t.Fatalf("report = %s, want products", ans.Plan.Report)
	}
	if ans.Envelope == nil || len(ans.Envelope.Rows) == 0 {
		t.Fatalf("ожидались строки товаров, envelope=%+v", ans.Envelope)
	}
}

func TestClarifyWhenNoPeriod(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "покажи выручку")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Validation.OK || !ans.Validation.NeedClarify {
		t.Fatalf("ожидался запрос уточнения периода: %+v", ans.Validation)
	}
	if ans.Envelope != nil {
		t.Error("envelope не должен формироваться без периода")
	}
	if !strings.Contains(ans.Text, "период") {
		t.Errorf("в уточнении нет слова 'период': %q", ans.Text)
	}
}

func TestTenantTimezoneInPeriod(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_tz", "выручка сегодня")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope: %+v", ans.Validation)
	}
	if ans.Envelope.Period.TZ != "Asia/Yekaterinburg" {
		t.Errorf("tz = %s, want Asia/Yekaterinburg", ans.Envelope.Period.TZ)
	}
}
