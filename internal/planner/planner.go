// Package planner — превращает произвольный текст в AnalysisPlan.
// LLMPlanner ходит в модель; Stub отдаёт детерминированные планы для тестов без GPU.
package planner

import (
	"context"

	"dgsbot/internal/catalog"
	"dgsbot/internal/plan"
	"dgsbot/internal/session"
)

// Planner — общий интерфейс планировщика.
// history — предыдущие реплики диалога (для follow-up и возобновления уточнений).
type Planner interface {
	Plan(ctx context.Context, history []session.Message, query string) (plan.AnalysisPlan, error)
}

// Catalog по умолчанию (4 MVP-отчёта) — общий для планировщиков.
func defaultCatalog() *catalog.Catalog { return catalog.Default() }
