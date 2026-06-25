package session

import (
	"testing"

	"dgsbot/internal/plan"
)

func TestAppendAndHistory(t *testing.T) {
	s := NewStore()
	s.Append("a", "вопрос1", "ответ1")
	s.Append("a", "вопрос2", "ответ2")

	h := s.History("a")
	if len(h) != 4 {
		t.Fatalf("ожидалось 4 реплики, got %d", len(h))
	}
	if h[0].Role != "user" || h[0].Content != "вопрос1" || h[1].Role != "assistant" {
		t.Errorf("неверный порядок/роли: %+v", h)
	}
}

func TestSessionsIsolated(t *testing.T) {
	s := NewStore()
	s.Append("a", "q", "r")
	if len(s.History("b")) != 0 {
		t.Error("сессии не изолированы")
	}
}

func TestHistoryTruncated(t *testing.T) {
	s := NewStore()
	for i := 0; i < MaxMessages; i++ {
		s.Append("a", "q", "r") // по 2 реплики за раз
	}
	if got := len(s.History("a")); got != MaxMessages {
		t.Errorf("история не обрезана до %d: got %d", MaxMessages, got)
	}
}

func TestHistoryIsCopy(t *testing.T) {
	s := NewStore()
	s.Append("a", "q", "r")
	h := s.History("a")
	h[0].Content = "mutated"
	if s.History("a")[0].Content != "q" {
		t.Error("History должна возвращать копию, а не ссылку на внутренний срез")
	}
}

func TestLastPlanRoundTrip(t *testing.T) {
	s := NewStore()
	_, ok := s.LastPlan("x")
	if ok {
		t.Fatal("LastPlan должен возвращать ok=false для новой сессии")
	}
	p := plan.AnalysisPlan{Report: "payment", Method: "plain"}
	s.SetLastPlan("x", p)
	got, ok := s.LastPlan("x")
	if !ok {
		t.Fatal("LastPlan должен вернуть ok=true после SetLastPlan")
	}
	if got.Report != "payment" {
		t.Errorf("Report=%q, want payment", got.Report)
	}
}

func TestLastPlanSessionIsolation(t *testing.T) {
	s := NewStore()
	s.SetLastPlan("a", plan.AnalysisPlan{Report: "payment"})
	_, ok := s.LastPlan("b")
	if ok {
		t.Error("last plan сессии a не должен быть виден в сессии b")
	}
}

func TestLastPlanOverwritten(t *testing.T) {
	s := NewStore()
	s.SetLastPlan("a", plan.AnalysisPlan{Report: "payment"})
	s.SetLastPlan("a", plan.AnalysisPlan{Report: "orders"})
	got, _ := s.LastPlan("a")
	if got.Report != "orders" {
		t.Errorf("ожидался перезаписанный план orders, got %q", got.Report)
	}
}
