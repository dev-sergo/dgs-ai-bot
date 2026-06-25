// Package querylog — дозапись датасета «вопрос → план → ответ» в JSONL-файл.
//
// На каждый запрос /ask пишется ровно одна строка JSON (см. Record). Файл нужен
// для продуктовой аналитики (какие запросы реально приходят) и для дообучения
// планировщика/нарратора на живых формулировках. Формат JSONL читается потоково:
//
//	jq -r .text data/queries.jsonl        # все вопросы пользователей
//	jq 'select(.outcome=="clarify")' …    # где бот переспрашивал
//
// Запись потокобезопасна; nil-приёмник означает «лог выключен» и тихо ничего не делает.
package querylog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"dgsbot/internal/plan"
)

// Record — одна строка датасета: что спросили, как поняли, что ответили.
type Record struct {
	TS        string            `json:"ts"`      // RFC3339, момент завершения обработки
	ID        string            `json:"id"`      // уникальный id ответа; совпадает с id в feedback.jsonl
	Tenant    string            `json:"tenant"`  // арендатор (X-Tenant-ID)
	Session   string            `json:"session"` // ключ диалоговой сессии
	Text      string            `json:"text"`    // исходный вопрос пользователя — как ввёл
	Intent    string            `json:"intent"`  // report|advice|help|smalltalk|off_topic
	Outcome   string            `json:"outcome"` // answer|clarify|empty|out_of_scope|error|…
	Plan      plan.AnalysisPlan `json:"plan"`    // распознанный план (как бот понял запрос)
	Answer    string            `json:"answer,omitempty"` // текст ответа, отданный пользователю
	Rows      int               `json:"rows"`             // строк в табличном ответе (0 — не таблица/пусто)
	LatencyMS int64             `json:"latency_ms"`       // время обработки запроса
	Err       string            `json:"err,omitempty"`    // текст ошибки, если ветка завершилась ошибкой
}

// Writer — потокобезопасная дозапись Record'ов в JSONL-файл (строка на запрос).
type Writer struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// Open открывает (создаёт) файл датасета на дозапись; каталог создаётся при необходимости.
// Существующий файл не перезатирается — новые строки добавляются в конец (переживает рестарт).
func Open(path string) (*Writer, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f, enc: json.NewEncoder(f)}, nil
}

// Write дозаписывает одну строку JSONL. Безопасно для nil-приёмника (лог выключен).
// Ошибка записи проглатывается осознанно: сбор датасета не должен ронять ответ пользователю.
func (w *Writer) Write(rec Record) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(rec) // Encode добавляет '\n' → ровно одна строка на запись
}

// Close закрывает файл. Безопасно для nil.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
