package planner

import (
	"context"
	"strings"

	"dgsbot/internal/plan"
)

// StubPlanner — детерминированный планировщик по ключевым словам.
// Нужен для интеграционных тестов и работы без доступа к LLM-ригу.
type StubPlanner struct{}

// NewStub создаёт стаб-планировщик.
func NewStub() *StubPlanner { return &StubPlanner{} }

func (s *StubPlanner) Plan(_ context.Context, query string) (plan.AnalysisPlan, error) {
	q := strings.ToLower(query)

	report := "payment"
	metrics := []string{"sum_all"}
	if strings.Contains(q, "товар") || strings.Contains(q, "продукт") || strings.Contains(q, "блюд") {
		report = "products"
		metrics = []string{"amount", "quantity"}
	}

	// Class B: «почему/за счёт» → contribution, «сравни/изменилась» → compare.
	isContribution := strings.Contains(q, "почему") || strings.Contains(q, "за счёт") || strings.Contains(q, "за счет")
	isCompare := strings.Contains(q, "сравни") || strings.Contains(q, "сравнен")
	if (isContribution || isCompare) && report == "payment" {
		method := "compare"
		if isContribution {
			method = "contribution"
		}
		tok := token(q)
		if tok == "" {
			tok = "last_30_days"
		}
		return plan.AnalysisPlan{
			Version: "1", Class: plan.ClassB, Report: "payment",
			Metrics:   []string{"sum_all"},
			Period:    plan.Period{Kind: "relative", Token: tok},
			CompareTo: &plan.Period{Kind: "relative", Token: "prev_period"},
			Method:    method, TopN: 5,
			Output:     plan.Output{Format: "text"},
			Confidence: 0.85,
		}, nil
	}

	p := plan.AnalysisPlan{
		Version: "1",
		Class:   plan.ClassA,
		Report:  report,
		Metrics: metrics,
		GroupBy: []string{},
		Period:  plan.Period{Kind: "relative", Token: token(q)},
		Method:  "plain",
		Output:  plan.Output{Format: "text"},
	}
	if report == "payment" {
		p.GroupBy = []string{"date"}
	} else {
		p.GroupBy = []string{"name"}
	}
	p.Confidence = 0.9
	return p, nil
}

func token(q string) string {
	switch {
	case strings.Contains(q, "вчера"):
		return "yesterday"
	case strings.Contains(q, "недел"):
		return "last_7_days"
	case strings.Contains(q, "месяц"):
		return "this_month"
	case strings.Contains(q, "сегодня"):
		return "today"
	default:
		return "" // период не распознан → валидатор попросит уточнить
	}
}
