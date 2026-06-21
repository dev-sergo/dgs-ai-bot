package integration

import (
	"context"
	"testing"

	"dgsbot/internal/pipeval"
)

// TestPipevalSet прогоняет full-pipeline набор (test/eval/pipeline.jsonl) через
// весь пайплайн app.Ask со StubPlanner + FixtureClient. Детерминированно, без рига.
// Проверяет ИТОГОВЫЙ ОТВЕТ (числа, текст, нарратив, отсутствие утечек), а не план.
func TestPipevalSet(t *testing.T) {
	cases, err := pipeval.LoadCases("../eval/pipeline.jsonl")
	if err != nil {
		t.Fatalf("загрузка набора: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("набор пуст")
	}

	a := newApp(t) // StubPlanner + FixtureClient + фиксированное «сейчас»
	results := pipeval.Run(context.Background(), a, "mock_single", cases)

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("[%s] ошибка: %v", r.Query, r.Err)
			continue
		}
		for _, m := range r.Mismatch {
			t.Errorf("[%s] %s", r.Query, m)
		}
	}
}
