package integration

import (
	"context"
	"strings"
	"testing"
)

func TestRevenueContribution(t *testing.T) {
	a := newApp(t) // StubPlanner: «почему» → contribution
	ans, err := a.Ask(context.Background(), "mock_single", "почему упала выручка за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope: %+v", ans.Validation)
	}
	if ans.Envelope.Type != "payment_contribution" {
		t.Fatalf("type = %s, want payment_contribution", ans.Envelope.Type)
	}
	// За июнь выручка ниже, чем за предыдущий период → снижение.
	if ans.Envelope.Summary["delta_abs"] >= 0 {
		t.Errorf("ожидалось снижение (delta_abs<0), got %v", ans.Envelope.Summary["delta_abs"])
	}
	if len(ans.Envelope.Rows) == 0 {
		t.Error("ожидалась раскладка по компонентам")
	}
	if !strings.Contains(ans.Text, "снизил") || !strings.Contains(ans.Text, "вклад") {
		t.Errorf("нарратив не объясняет снижение/вклад:\n%s", ans.Text)
	}
}

func TestRevenueCompare(t *testing.T) {
	a := newApp(t) // StubPlanner: «сравни» → compare
	ans, err := a.Ask(context.Background(), "mock_single", "сравни выручку за неделю")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil || ans.Envelope.Type != "payment_compare" {
		t.Fatalf("ожидался payment_compare: %+v", ans.Validation)
	}
	if _, ok := ans.Envelope.Summary["delta_pct"]; !ok {
		t.Error("в summary нет delta_pct")
	}
	if ans.Text == "" {
		t.Error("ожидался нарратив сравнения")
	}
}
