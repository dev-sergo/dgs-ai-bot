package eval

import (
	"testing"

	"dgsbot/internal/plan"
)

func TestCheckReportMismatch(t *testing.T) {
	p := plan.AnalysisPlan{Report: "products", Method: "plain"}
	m := Check(p, Expect{Report: "payment"})
	if len(m) == 0 {
		t.Fatal("ожидалось расхождение по report")
	}
}

func TestCheckPasses(t *testing.T) {
	p := plan.AnalysisPlan{
		Report: "payment", Class: plan.ClassB, Method: "contribution",
		Period:  plan.Period{Token: "last_30_days"},
		Filters: []plan.Filter{{Field: "sale_point", Values: []string{"Выкса"}}},
	}
	m := Check(p, Expect{Report: "payment", Class: "B", Method: "contribution",
		PeriodToken: "last_30_days", Filters: []string{"sale_point"}})
	if len(m) != 0 {
		t.Fatalf("ожидалось совпадение, получены расхождения: %v", m)
	}
}

func TestCheckMissingFilter(t *testing.T) {
	p := plan.AnalysisPlan{Report: "payment"}
	m := Check(p, Expect{Filters: []string{"sale_point"}})
	if len(m) == 0 {
		t.Fatal("ожидалось расхождение по отсутствующему фильтру")
	}
}

func TestCheckFilterValues(t *testing.T) {
	withFilter := func(field string, vals ...string) plan.AnalysisPlan {
		return plan.AnalysisPlan{
			Report:  "payment",
			Filters: []plan.Filter{{Field: field, Values: vals}},
		}
	}

	tests := []struct {
		name    string
		plan    plan.AnalysisPlan
		expect  Expect
		wantErr bool
	}{
		{
			name:    "exact match",
			plan:    withFilter("sale_point", "Выкса"),
			expect:  Expect{FilterValues: []FilterExpect{{Field: "sale_point", Values: []string{"Выкса"}}}},
			wantErr: false,
		},
		{
			name:    "case-insensitive match",
			plan:    withFilter("sale_point", "выкса"),
			expect:  Expect{FilterValues: []FilterExpect{{Field: "sale_point", Values: []string{"Выкса"}}}},
			wantErr: false,
		},
		{
			name:    "alternative values — first matches",
			plan:    withFilter("user", "Петров"),
			expect:  Expect{FilterValues: []FilterExpect{{Field: "user", Values: []string{"Петров", "Петрова"}}}},
			wantErr: false,
		},
		{
			name:    "alternative values — second matches",
			plan:    withFilter("user", "Петрова"),
			expect:  Expect{FilterValues: []FilterExpect{{Field: "user", Values: []string{"Петров", "Петрова"}}}},
			wantErr: false,
		},
		{
			name:    "value mismatch",
			plan:    withFilter("sale_point", "Центральная"),
			expect:  Expect{FilterValues: []FilterExpect{{Field: "sale_point", Values: []string{"Выкса"}}}},
			wantErr: true,
		},
		{
			name:    "filter field missing",
			plan:    plan.AnalysisPlan{Report: "payment"},
			expect:  Expect{FilterValues: []FilterExpect{{Field: "sale_point", Values: []string{"Выкса"}}}},
			wantErr: true,
		},
		{
			name: "multi-filter both present and correct",
			plan: plan.AnalysisPlan{
				Report: "payment",
				Filters: []plan.Filter{
					{Field: "sale_point", Values: []string{"Выкса"}},
					{Field: "user", Values: []string{"Петров"}},
				},
			},
			expect: Expect{FilterValues: []FilterExpect{
				{Field: "sale_point", Values: []string{"Выкса"}},
				{Field: "user", Values: []string{"Петров"}},
			}},
			wantErr: false,
		},
		{
			name: "multi-filter second field missing",
			plan: plan.AnalysisPlan{
				Report:  "payment",
				Filters: []plan.Filter{{Field: "sale_point", Values: []string{"Выкса"}}},
			},
			expect: Expect{FilterValues: []FilterExpect{
				{Field: "sale_point", Values: []string{"Выкса"}},
				{Field: "user", Values: []string{"Петров"}},
			}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Check(tc.plan, tc.expect)
			if tc.wantErr && len(m) == 0 {
				t.Fatal("ожидалось расхождение, получено совпадение")
			}
			if !tc.wantErr && len(m) != 0 {
				t.Fatalf("ожидалось совпадение, получены расхождения: %v", m)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	rs := []Result{
		{Valid: true, LatencyMS: 100},                          // pass
		{Valid: true, Mismatch: []string{"x"}, LatencyMS: 200}, // fail
		{Err: errFake{}, LatencyMS: 300},                       // err
	}
	s := Summarize(rs)
	if s.Total != 3 || s.Passed != 1 || s.Errors != 1 || s.Valid != 2 {
		t.Fatalf("неверная сводка: %+v", s)
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake" }
