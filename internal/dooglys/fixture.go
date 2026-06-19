package dooglys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FixtureClient отдаёт данные из нормализованных фикстур (docs/contracts/fixtures/<report>.json).
// Если у строк есть поле "date" (ISO) — фильтрует по периоду запроса; иначе возвращает снимок целиком.
type FixtureClient struct {
	dir string
}

// NewFixtureClient создаёт клиента поверх каталога фикстур.
func NewFixtureClient(dir string) *FixtureClient { return &FixtureClient{dir: dir} }

type fixtureFile struct {
	Report string `json:"report"`
	Label  string `json:"label"`
	Rows   []Row  `json:"rows"`
}

const (
	isoLayout = "2006-01-02"
	ruLayout  = "02.01.2006"
)

func (c *FixtureClient) Fetch(_ context.Context, q Query) (Result, error) {
	path := filepath.Join(c.dir, q.Report+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("фикстура отчёта %q не найдена: %w", q.Report, err)
	}
	var ff fixtureFile
	if err := json.Unmarshal(raw, &ff); err != nil {
		return Result{}, fmt.Errorf("разбор фикстуры %q: %w", q.Report, err)
	}

	rows := ff.Rows
	if q.From != "" && q.To != "" {
		rows = filterByPeriod(rows, q.From, q.To)
	}
	return Result{Report: ff.Report, Label: ff.Label, Rows: rows}, nil
}

// filterByPeriod оставляет строки, у которых поле date попадает в [from..to].
// Строки без поля date возвращаются без фильтрации (снимок за весь период).
func filterByPeriod(rows []Row, from, to string) []Row {
	fromT, err1 := time.Parse(ruLayout, from)
	toT, err2 := time.Parse(ruLayout, to)
	if err1 != nil || err2 != nil {
		return rows
	}
	hasDate := false
	for _, r := range rows {
		if _, ok := r["date"]; ok {
			hasDate = true
			break
		}
	}
	if !hasDate {
		return rows
	}

	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		ds, ok := r["date"].(string)
		if !ok {
			continue
		}
		d, err := time.Parse(isoLayout, ds)
		if err != nil {
			continue
		}
		if !d.Before(fromT) && !d.After(toT) {
			out = append(out, r)
		}
	}
	return out
}
