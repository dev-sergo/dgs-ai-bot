package querylog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteAppendsJSONL: каждая запись — отдельная валидная JSON-строка, дозапись в конец.
func TestWriteAppendsJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "queries.jsonl") // каталог ещё не существует
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w.Write(Record{Text: "выручка за май", Intent: "report", Outcome: "answer"})
	w.Write(Record{Text: "привет", Intent: "smalltalk", Outcome: "smalltalk"})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open written file: %v", err)
	}
	defer f.Close()

	var texts []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("строка не парсится как JSON: %q: %v", sc.Text(), err)
		}
		texts = append(texts, rec.Text)
	}
	if len(texts) != 2 || texts[0] != "выручка за май" || texts[1] != "привет" {
		t.Fatalf("ожидали 2 строки [выручка за май, привет], получили %v", texts)
	}
}

// TestReopenAppends: повторный Open того же файла не затирает прежние строки (переживает рестарт).
func TestReopenAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	w1, _ := Open(path)
	w1.Write(Record{Text: "первый"})
	w1.Close()

	w2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	w2.Write(Record{Text: "второй"})
	w2.Close()

	data, _ := os.ReadFile(path)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("ожидали 2 строки после reopen, насчитали %d; содержимое:\n%s", lines, data)
	}
}

// TestNilWriterIsNoop: nil-приёмник (лог выключен) не паникует на Write/Close.
func TestNilWriterIsNoop(t *testing.T) {
	var w *Writer
	w.Write(Record{Text: "ничего не должно произойти"})
	if err := w.Close(); err != nil {
		t.Fatalf("Close на nil вернул ошибку: %v", err)
	}
}
