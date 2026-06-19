// Package planner — превращает произвольный текст в AnalysisPlan.
// LLMPlanner ходит в модель; Stub отдаёт детерминированные планы для тестов без GPU.
package planner

import (
	"context"

	"dgsbot/internal/catalog"
	"dgsbot/internal/plan"
)

// Planner — общий интерфейс планировщика.
type Planner interface {
	Plan(ctx context.Context, query string) (plan.AnalysisPlan, error)
}

// Catalog по умолчанию (4 MVP-отчёта) — общий для планировщиков.
func defaultCatalog() *catalog.Catalog { return catalog.Default() }
