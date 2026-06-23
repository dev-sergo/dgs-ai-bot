package integration

import (
	"context"
	"strings"
	"testing"

	"dgsbot/internal/plan"
	"dgsbot/internal/session"
)

// scriptPlanner отдаёт планы по очереди (по одному на вызов Plan), последний — залипает.
// Моделирует диалог: сначала advice без периода, затем «голый» период-ответ.
type scriptPlanner struct {
	plans []plan.AnalysisPlan
	i     int
}

func (s *scriptPlanner) Plan(_ context.Context, _ []session.Message, _ string) (plan.AnalysisPlan, error) {
	p := s.plans[s.i]
	if s.i < len(s.plans)-1 {
		s.i++
	}
	return p, nil
}

// Регресс на баг: advice спросил период → пользователь ответил периодом →
// раньше планировщик терял intent=advice и выдавал обычный отчёт. Теперь — продолжает разбор.
func TestAdviceResumesAfterPeriodClarify(t *testing.T) {
	advice := plan.AnalysisPlan{Version: "1", Intent: "advice", Report: "payment", Method: "plain"}
	// Ответ-период приходит как обычный report-план с периодом (intent НЕ advice — в этом и баг).
	periodReply := plan.AnalysisPlan{
		Version: "1", Intent: "report", Report: "payment", Metrics: []string{"sum_all"},
		Period: plan.Period{Kind: "relative", Token: "last_30_days"}, Method: "plain",
	}
	a := newAppWith(t, &scriptPlanner{plans: []plan.AnalysisPlan{advice, periodReply}})

	// Ход 1: advice без периода → переспрос периода.
	ans1, err := a.Ask(context.Background(), "mock_single", "s", "на чём я теряю деньги")
	if err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if !ans1.Validation.NeedClarify {
		t.Fatalf("ход 1: ждали переспрос периода, got %+v", ans1.Validation)
	}

	// Ход 2: «за последние 30 дней» → должен возобновиться advice, а не плоский отчёт.
	ans2, err := a.Ask(context.Background(), "mock_single", "s", "за последние 30 дней")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if ans2.Plan.Intent != "advice" {
		t.Fatalf("ход 2: intent=%q, want advice (период не должен сбрасывать разбор)", ans2.Plan.Intent)
	}
	if ans2.Validation.NeedClarify {
		t.Fatalf("ход 2: неожиданный повторный clarify: %q", ans2.Text)
	}
	if strings.TrimSpace(ans2.Text) == "" {
		t.Fatal("ход 2: пустой текст разбора")
	}
}
