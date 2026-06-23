package integration

import (
	"context"
	"strings"
	"testing"

	"dgsbot/internal/plan"
)

// midConfPlan — валидный план выручки за неделю со средней уверенностью (полоса
// подтверждения [0.5, 0.7)). fixedPlanner отдаёт его на любой текст.
func midConfPlan() plan.AnalysisPlan {
	return plan.AnalysisPlan{
		Version: "1", Intent: "report", Class: plan.ClassA,
		Report: "payment", Metrics: []string{"sum_all"}, GroupBy: []string{"date"},
		Period: plan.Period{Kind: "relative", Token: "last_7_days"},
		Method: "plain", Output: plan.Output{Format: "text"},
		Confidence: 0.6,
	}
}

// Средняя уверенность → бот сначала переспрашивает интерпретацию, потом исполняет по «да».
func TestPlanConfirmThenExecute(t *testing.T) {
	a := newAppWith(t, fixedPlanner{p: midConfPlan()})
	ctx := context.Background()

	// Ход 1: эхо интерпретации, без отчёта.
	ans, err := a.Ask(ctx, "mock_single", "s", "сколько заработали")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil {
		t.Fatal("на средней уверенности отчёт сразу строить нельзя — нужен confirm")
	}
	if !ans.Validation.NeedClarify || !strings.Contains(ans.Text, "Правильно понимаю") {
		t.Fatalf("ожидался эхо-переспрос, got: %+v / %q", ans.Validation, ans.Text)
	}
	if !strings.Contains(ans.Text, "Выручка") {
		t.Errorf("эхо должно называть отчёт: %q", ans.Text)
	}

	// Ход 2: «да» → исполняем сохранённый план.
	ans, err = a.Ask(ctx, "mock_single", "s", "да")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("после «да» ожидался отчёт, got: %q", ans.Text)
	}
	if ans.Envelope.Summary["value_now"] == 0 && len(ans.Envelope.Rows) == 0 {
		t.Error("отчёт пустой — план не исполнился")
	}
}

// Не-подтверждение сбрасывает pending и НЕ исполняет устаревший план (перепланируется).
func TestPlanConfirmNonAffirmationReplans(t *testing.T) {
	a := newAppWith(t, fixedPlanner{p: midConfPlan()})
	ctx := context.Background()

	if _, err := a.Ask(ctx, "mock_single", "s", "сколько заработали"); err != nil {
		t.Fatal(err)
	}
	// «нет» — не подтверждение: устаревший план не исполняется, идём обычным путём
	// (fixedPlanner снова отдаёт средне-уверенный план → снова confirm, не отчёт).
	ans, err := a.Ask(ctx, "mock_single", "s", "нет")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil {
		t.Fatal("«нет» не должно исполнять отложенный план")
	}
	if !strings.Contains(ans.Text, "Правильно понимаю") {
		t.Errorf("ожидался повторный переспрос, got: %q", ans.Text)
	}

	// «да» теперь подтверждает свежий pending → отчёт.
	ans, err = a.Ask(ctx, "mock_single", "s", "да")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("после «да» ожидался отчёт, got: %q", ans.Text)
	}
}

// Без pending одинокое «да» не превращается в отчёт (нечего подтверждать).
func TestBareAffirmationNoPending(t *testing.T) {
	a := newAppWith(t, fixedPlanner{p: midConfPlan()})
	ans, err := a.Ask(context.Background(), "mock_single", "fresh", "да")
	if err != nil {
		t.Fatal(err)
	}
	// «да» планируется как обычный текст → средняя уверенность → confirm, а не отчёт.
	if ans.Envelope != nil {
		t.Fatal("одинокое «да» без ожидающего плана не должно строить отчёт")
	}
}

// Высокая уверенность — исполняем сразу, без лишнего переспроса (полоса не задевает).
func TestHighConfidenceNoConfirm(t *testing.T) {
	p := midConfPlan()
	p.Confidence = 0.9
	a := newAppWith(t, fixedPlanner{p: p})
	ans, err := a.Ask(context.Background(), "mock_single", "s", "выручка за неделю")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("высокая уверенность — отчёт сразу, got: %q", ans.Text)
	}
}
