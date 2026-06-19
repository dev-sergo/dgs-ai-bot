package session

import "testing"

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
