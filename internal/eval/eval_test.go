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

func TestSummarize(t *testing.T) {
	rs := []Result{
		{Valid: true, LatencyMS: 100},                 // pass
		{Valid: true, Mismatch: []string{"x"}, LatencyMS: 200}, // fail
		{Err: errFake{}, LatencyMS: 300},              // err
	}
	s := Summarize(rs)
	if s.Total != 3 || s.Passed != 1 || s.Errors != 1 || s.Valid != 2 {
		t.Fatalf("неверная сводка: %+v", s)
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake" }
