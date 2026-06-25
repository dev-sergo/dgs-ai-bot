// Package feedback — дозапись оценок пользователя (👍/👎) в JSONL-файл.
//
// Один тап → одна строка JSON. Связь с запросом — по полю id (то же id, что
// в queries.jsonl). Для анализа качества:
//
//	jq -r .id data/feedback.jsonl | while read id; do
//	  jq --arg id "$id" 'select(.id==$id)' data/queries.jsonl
//	done
package feedback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Record — одна строка фидбэка: какой ответ оценили, как и откуда.
type Record struct {
	TS     string `json:"ts"`     // RFC3339, момент тапа
	ID     string `json:"id"`     // id ответа из queries.jsonl
	Rating string `json:"rating"` // "up" | "down"
	Source string `json:"source"` // "ui" | "telegram"
}

// Writer — потокобезопасная дозапись Record'ов в JSONL-файл.
type Writer struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// Open открывает (создаёт) файл фидбэка на дозапись; каталог создаётся при необходимости.
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

// Write дозаписывает одну строку JSONL. Безопасно для nil-приёмника.
func (w *Writer) Write(rec Record) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(rec)
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
