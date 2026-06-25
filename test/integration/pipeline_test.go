// Package integration — сквозные тесты пайплайна без GPU: StubPlanner + FixtureClient.
package integration

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"dgsbot/internal/advisor"
	"dgsbot/internal/app"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/narrator"
	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
	"dgsbot/internal/resolver"
	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
)

const fixturesDir = "../../docs/contracts/fixtures"

func newAppStore(t *testing.T, pl planner.Planner) (*app.App, *session.Store) {
	t.Helper()
	tenants, err := tenantctx.Load(fixturesDir + "/tenants.example.json")
	if err != nil {
		t.Fatalf("load tenants: %v", err)
	}
	store := session.NewStore()
	a := app.New(pl, tenants, dooglys.NewFixtureClient(fixturesDir), resolver.Load(fixturesDir), narrator.NewTemplate(), advisor.NewTemplate(), store)
	// Фиксированное «сейчас» для детерминизма дат: 2026-06-19 10:00 UTC.
	a.Now = func() time.Time { return time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC) }
	// Тихий логгер — чтобы аудит-строки Ask не зашумляли вывод тестов.
	a.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return a, store
}

func newAppWith(t *testing.T, pl planner.Planner) *app.App {
	a, _ := newAppStore(t, pl)
	return a
}

func newApp(t *testing.T) *app.App { return newAppWith(t, planner.NewStub()) }

func TestRevenueLastWeek(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "покажи выручку за последнюю неделю")
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
	ans, err := a.Ask(context.Background(), "mock_single", "s", "какие товары продавались за месяц")
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

func TestBestVsWorstProductsDiffer(t *testing.T) {
	a := newApp(t)
	best, err := a.Ask(context.Background(), "mock_single", "s", "какие товары продаются лучше всего за месяц")
	if err != nil {
		t.Fatal(err)
	}
	worst, err := a.Ask(context.Background(), "mock_single", "s2", "какие товары продаются хуже всего за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if best.Plan.Method != "top_n" || worst.Plan.Method != "top_n" {
		t.Fatalf("ожидался метод top_n: best=%s worst=%s", best.Plan.Method, worst.Plan.Method)
	}
	if best.Envelope == nil || worst.Envelope == nil || len(best.Envelope.Rows) == 0 || len(worst.Envelope.Rows) == 0 {
		t.Fatal("ожидались непустые рейтинги")
	}
	// Лучший по amount должен быть отсортирован убывающе, худший — возрастающе.
	bestTop, _ := best.Envelope.Rows[0]["amount"].(float64)
	bestSecond, _ := best.Envelope.Rows[1]["amount"].(float64)
	if bestTop < bestSecond {
		t.Errorf("лучшие не по убыванию: %v < %v", bestTop, bestSecond)
	}
	worstTop, _ := worst.Envelope.Rows[0]["amount"].(float64)
	if worstTop > bestTop {
		t.Errorf("худший (%v) не должен превышать лучшего (%v)", worstTop, bestTop)
	}
	// Первая строка «лучших» и «худших» не должна совпадать (раньше были идентичны).
	if best.Envelope.Rows[0]["name"] == worst.Envelope.Rows[0]["name"] {
		t.Errorf("лучший и худший товар совпали: %v", best.Envelope.Rows[0]["name"])
	}
}

func TestProductsAggregatedByName(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "топ товаров за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatal("нет envelope")
	}
	seen := map[string]bool{}
	for _, r := range ans.Envelope.Rows {
		name, _ := r["name"].(string)
		if seen[name] {
			t.Errorf("дубль товара после агрегации: %q", name)
		}
		seen[name] = true
	}
}

func TestSingleBestProduct(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "какой товар самый популярный за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil || len(ans.Envelope.Rows) != 1 {
		t.Fatalf("ожидалась ровно 1 строка (топ-1), got %d", len(ans.Envelope.Rows))
	}
}

func TestClarifyWhenNoPeriod(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "покажи выручку")
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
	// Период с данными (за неделю) — чтобы envelope сформировался и можно было
	// проверить таймзону. «сегодня» в фикстурах пусто → envelope не отдаётся.
	ans, err := a.Ask(context.Background(), "mock_tz", "s", "выручка за неделю")
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

// Консультационный запрос («на чём теряю») → режим advice: текст-разбор без envelope,
// с числами потерь (возвраты/скидки). Stub проставляет период, RefineAdvice — intent=advice.
func TestAdviceLosses(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "на чём я теряю деньги за прошлый месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Plan.Intent != "advice" {
		t.Fatalf("intent = %q, want advice", ans.Plan.Intent)
	}
	if ans.Text == "" {
		t.Fatal("ожидался текст совета")
	}
	// Детерминированный Compose (stub-режим) называет драйверы потерь.
	if !strings.Contains(ans.Text, "Выручка") {
		t.Errorf("в совете нет контекста выручки:\n%s", ans.Text)
	}
}

// Консультация с фильтром по точке: фильтр прокидывается в снимок, но фикстуры
// payment/products не несут колонку точки → честный отказ, а НЕ снимок по всему
// заведению под видом среза. Так advice наследует safety главного пути (Этап A).
func TestAdviceFilterUnsupportedScopeIsHonest(t *testing.T) {
	pl := fixedPlanner{p: plan.AnalysisPlan{
		Intent: "advice", Report: "payment", Method: "plain",
		Metrics: []string{"sum_all"},
		Period:  plan.Period{Kind: "relative", Token: "this_month"},
		Filters: []plan.Filter{{Field: "sale_point", Values: []string{"Казанский вокзал"}}},
	}}
	a := newAppWith(t, pl)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "что улучшить на точке Казанский вокзал за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Validation.OK {
		t.Fatalf("ожидался честный отказ по неподдержанному разрезу, got OK; текст: %q", ans.Text)
	}
	if !strings.Contains(ans.Text, "не поддерживает разрез") {
		t.Errorf("ожидалось сообщение о неподдержанном разрезе, got: %q", ans.Text)
	}
}

// Консультация без периода → переспрос периода, а не угадывание.
func TestAdviceNeedsPeriod(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "на чём я теряю деньги")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Plan.Intent != "advice" {
		t.Fatalf("intent = %q, want advice", ans.Plan.Intent)
	}
	if !ans.Validation.NeedClarify {
		t.Errorf("ожидался переспрос периода, got: %q", ans.Text)
	}
}
