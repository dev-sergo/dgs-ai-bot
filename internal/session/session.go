// Package session — память диалога по сессиям (для многоходового общения).
//
// В MVP — in-memory с ограничением длины истории. В проде заменяется на Redis
// (ключ — session_id из пре-слоя авторизации) без изменения интерфейса.
package session

import (
	"sync"

	"dgsbot/internal/plan"
)

// Message — реплика диалога.
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// MaxMessages — сколько последних реплик храним и отдаём модели в контекст.
// Ограничение бережёт окно контекста (ctx модели) и латентность.
const MaxMessages = 12

// Store — потокобезопасное хранилище истории по session_id.
type Store struct {
	mu       sync.Mutex
	data     map[string][]Message
	pending  map[string]plan.AnalysisPlan // план, ждущий подтверждения «да» (plan-confirm)
	awaiting map[string]plan.AnalysisPlan // advice-план, ждущий ответа про период (clarify-resume)
	last     map[string]plan.AnalysisPlan // последний успешно исполненный план (carry-over для follow-up)
}

// NewStore создаёт пустое хранилище.
func NewStore() *Store {
	return &Store{
		data:     map[string][]Message{},
		pending:  map[string]plan.AnalysisPlan{},
		awaiting: map[string]plan.AnalysisPlan{},
		last:     map[string]plan.AnalysisPlan{},
	}
}

// History возвращает копию истории сессии (последние MaxMessages реплик).
func (s *Store) History(sessionID string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.data[sessionID]
	out := make([]Message, len(h))
	copy(out, h)
	return out
}

// Append добавляет пару «запрос пользователя → ответ ассистента» и обрезает историю.
func (s *Store) Append(sessionID, userText, assistantText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := append(s.data[sessionID],
		Message{Role: "user", Content: userText},
		Message{Role: "assistant", Content: assistantText},
	)
	if len(h) > MaxMessages {
		h = h[len(h)-MaxMessages:]
	}
	s.data[sessionID] = h
}

// SetPending запоминает план, который ждёт подтверждения пользователя («да»).
// Однократный: следующая реплика его забирает (TakePending) независимо от ответа.
func (s *Store) SetPending(sessionID string, p plan.AnalysisPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[sessionID] = p
}

// TakePending возвращает и удаляет ожидающий план (single-shot). ok=false, если его нет.
func (s *Store) TakePending(sessionID string) (plan.AnalysisPlan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[sessionID]
	if ok {
		delete(s.pending, sessionID)
	}
	return p, ok
}

// SetAwaitingPeriod запоминает advice-план, для которого спросили период.
// Следующая реплика-период возобновит консультацию тем же планом (а не упадёт в report).
func (s *Store) SetAwaitingPeriod(sessionID string, p plan.AnalysisPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.awaiting[sessionID] = p
}

// TakeAwaitingPeriod возвращает и удаляет ожидающий период advice-план (single-shot).
func (s *Store) TakeAwaitingPeriod(sessionID string) (plan.AnalysisPlan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.awaiting[sessionID]
	if ok {
		delete(s.awaiting, sessionID)
	}
	return p, ok
}

// SetLastPlan запоминает последний успешно исполненный план отчёта. Перезаписывается
// при каждом успешном ответе; используется для детерминированного переноса фильтров/
// периода в follow-up («а по карте?», «а за прошлую неделю?»).
func (s *Store) SetLastPlan(sessionID string, p plan.AnalysisPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last[sessionID] = p
}

// LastPlan возвращает последний успешно исполненный план. ok=false, если его нет.
func (s *Store) LastPlan(sessionID string) (plan.AnalysisPlan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.last[sessionID]
	return p, ok
}
