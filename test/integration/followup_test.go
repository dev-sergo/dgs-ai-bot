package integration

import (
	"context"
	"testing"

	"dgsbot/internal/plan"
	"dgsbot/internal/session"
)

// stubSeqPlanner возвращает планы последовательно из очереди (по одному на каждый вызов Plan).
type stubSeqPlanner struct {
	plans []plan.AnalysisPlan
	idx   int
}

func (s *stubSeqPlanner) Plan(_ context.Context, _ []session.Message, _ string) (plan.AnalysisPlan, error) {
	p := s.plans[s.idx]
	if s.idx < len(s.plans)-1 {
		s.idx++
	}
	return p, nil
}

func ordersWeekPlan(filters ...plan.Filter) plan.AnalysisPlan {
	return plan.AnalysisPlan{
		Version: "1", Class: plan.ClassA, Report: "orders",
		Metrics: []string{"paid"}, GroupBy: []string{"torgovaya_tochka"},
		Period:  plan.Period{Kind: "relative", Token: "last_30_days"},
		Method:  "plain", Output: plan.Output{Format: "text"},
		Filters: filters,
		Intent:  "report",
	}
}

func ordersThinPlan(filters ...plan.Filter) plan.AnalysisPlan {
	return plan.AnalysisPlan{
		Version: "1", Class: plan.ClassA, Report: "orders",
		Metrics: []string{"paid"}, GroupBy: []string{"torgovaya_tochka"},
		Period:  plan.Period{}, // нет периода — сигнал уточняющей реплики
		Method:  "plain", Output: plan.Output{Format: "text"},
		Filters: filters,
		Intent:  "report",
	}
}

// «заказы Выкса за месяц» → «а за прошлую неделю?» (период пуст, фильтр забыт):
// period и sale_point=Выкса переносятся из последнего плана.
func TestFollowUpPeriodCarried(t *testing.T) {
	pl := &stubSeqPlanner{plans: []plan.AnalysisPlan{
		ordersWeekPlan(plan.Filter{Field: "sale_point", Op: "in", Values: []string{"Выкса"}}),
		ordersThinPlan(), // период пуст, sale_point отсутствует
	}}
	a := newAppWith(t, pl)
	ctx := context.Background()
	sid := "test-followup-period"

	ans1, err := a.Ask(ctx, "mock_single", sid, "заказы Выкса за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if !ans1.Validation.OK {
		t.Fatalf("первый ход должен быть OK: %+v", ans1.Validation)
	}

	ans2, err := a.Ask(ctx, "mock_single", sid, "а за прошлую неделю?")
	if err != nil {
		t.Fatal(err)
	}
	// period перенёсся из last плана
	if ans2.Plan.Period.Token != "last_30_days" {
		t.Errorf("period не перенёсся: got %+v", ans2.Plan.Period)
	}
	// sale_point фильтр перенёсся
	hasSP := false
	for _, f := range ans2.Plan.Filters {
		if f.Field == "sale_point" {
			hasSP = true
		}
	}
	if !hasSP {
		t.Errorf("фильтр sale_point не перенёсся: filters=%+v", ans2.Plan.Filters)
	}
}

// «заказы Выкса за месяц» → «а по карте?» (модель ставит payment_type=card, период пуст):
// period переносится И sale_point=Выкса мержится к payment_type.
func TestFollowUpFilterMerged(t *testing.T) {
	pl := &stubSeqPlanner{plans: []plan.AnalysisPlan{
		ordersWeekPlan(plan.Filter{Field: "sale_point", Op: "in", Values: []string{"Выкса"}}),
		// второй ход: модель ставит payment_type (enum, valid для orders), период пуст
		ordersThinPlan(plan.Filter{Field: "payment_type", Op: "in", Values: []string{"card"}}),
	}}
	a := newAppWith(t, pl)
	ctx := context.Background()
	sid := "test-followup-merge"

	ans1, err := a.Ask(ctx, "mock_single", sid, "заказы Выкса за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if !ans1.Validation.OK {
		t.Fatalf("первый ход должен быть OK: %+v", ans1.Validation)
	}

	ans2, err := a.Ask(ctx, "mock_single", sid, "а по карте?")
	if err != nil {
		t.Fatal(err)
	}
	// period перенёсся
	if ans2.Plan.Period.Token != "last_30_days" {
		t.Errorf("period не перенёсся: got %+v", ans2.Plan.Period)
	}
	// оба фильтра присутствуют в плане
	fields := map[string]bool{}
	for _, f := range ans2.Plan.Filters {
		fields[f.Field] = true
	}
	if !fields["payment_type"] {
		t.Errorf("payment_type фильтр потерялся: filters=%+v", ans2.Plan.Filters)
	}
	if !fields["sale_point"] {
		t.Errorf("sale_point не перенёсся при merge: filters=%+v", ans2.Plan.Filters)
	}
}

// Если у второго хода есть собственный период — carry-over фильтров НЕ срабатывает:
// новый независимый запрос не должен тащить фильтры предыдущего.
func TestFollowUpNoCarryWhenOwnPeriod(t *testing.T) {
	pl := &stubSeqPlanner{plans: []plan.AnalysisPlan{
		ordersWeekPlan(plan.Filter{Field: "sale_point", Op: "in", Values: []string{"Выкса"}}),
		ordersWeekPlan(), // свой period, нет фильтра — новый независимый запрос
	}}
	a := newAppWith(t, pl)
	ctx := context.Background()
	sid := "test-followup-nocarry"

	if _, err := a.Ask(ctx, "mock_single", sid, "заказы Выкса за месяц"); err != nil {
		t.Fatal(err)
	}

	ans2, err := a.Ask(ctx, "mock_single", sid, "заказы за месяц")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ans2.Plan.Filters {
		if f.Field == "sale_point" {
			t.Errorf("sale_point НЕ должен переноситься при наличии своего периода: filters=%+v", ans2.Plan.Filters)
		}
	}
}
