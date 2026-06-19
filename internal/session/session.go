// Package session — память диалога по сессиям (для многоходового общения).
//
// В MVP — in-memory с ограничением длины истории. В проде заменяется на Redis
// (ключ — session_id из пре-слоя авторизации) без изменения интерфейса.
package session

import "sync"

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
	mu   sync.Mutex
	data map[string][]Message
}

// NewStore создаёт пустое хранилище.
func NewStore() *Store { return &Store{data: map[string][]Message{}} }

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
