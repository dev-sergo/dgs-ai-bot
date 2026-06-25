package feedback

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriter_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "feedback.jsonl")

	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	w.Write(Record{TS: "2026-06-25T10:00:00Z", ID: "abc123", Rating: "down", Source: "ui"})
	w.Write(Record{TS: "2026-06-25T10:01:00Z", ID: "def456", Rating: "up", Source: "telegram"})

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open result: %v", err)
	}
	defer f.Close()

	var recs []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		recs = append(recs, r)
	}

	if len(recs) != 2 {
		t.Fatalf("ожидали 2 записи, получили %d", len(recs))
	}
	if recs[0].ID != "abc123" || recs[0].Rating != "down" || recs[0].Source != "ui" {
		t.Errorf("запись 0 не совпадает: %+v", recs[0])
	}
	if recs[1].ID != "def456" || recs[1].Rating != "up" || recs[1].Source != "telegram" {
		t.Errorf("запись 1 не совпадает: %+v", recs[1])
	}
}

func TestWriter_NilSafe(t *testing.T) {
	var w *Writer
	w.Write(Record{ID: "x", Rating: "up"}) // не должно паниковать
	if err := w.Close(); err != nil {
		t.Errorf("nil Close: %v", err)
	}
}
