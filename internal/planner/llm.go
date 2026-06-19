package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dgsbot/internal/catalog"
	"dgsbot/internal/llm"
	"dgsbot/internal/plan"
)

// LLMPlanner строит AnalysisPlan через модель с guided-JSON.
type LLMPlanner struct {
	cli   *llm.Client
	model string
	cat   *catalog.Catalog
}

// NewLLM создаёт планировщик поверх LLM-клиента.
func NewLLM(cli *llm.Client, model string) *LLMPlanner {
	return &LLMPlanner{cli: cli, model: model, cat: defaultCatalog()}
}

func (p *LLMPlanner) Plan(ctx context.Context, query string) (plan.AnalysisPlan, error) {
	msgs := []llm.Message{
		{Role: "system", Content: p.systemPrompt()},
		{Role: "user", Content: query},
	}
	raw, err := p.cli.Chat(ctx, p.model, msgs, llm.ChatOptions{Temperature: 0, JSONObject: true})
	if err != nil {
		return plan.AnalysisPlan{}, err
	}
	return parsePlan(raw)
}

func (p *LLMPlanner) systemPrompt() string {
	return `Ты — планировщик аналитических запросов для системы отчётов кафе.
Твоя задача: превратить запрос владельца в строгий JSON-план. Ты НЕ считаешь числа и НЕ пишешь текст ответа.

Доступные отчёты и поля (white-list — использовать ТОЛЬКО их):
` + p.cat.Describe() + `
Верни СТРОГО один JSON-объект со схемой:
{
  "version": "1",
  "class": "A" | "B",            // A — простой отчёт, B — аналитика (сравнение/вклад)
  "report": "<slug отчёта>",
  "metrics": ["<ключи полей>"],
  "group_by": ["<ключи полей>"],
  "period": {"kind":"relative","token":"today|yesterday|last_7_days|last_30_days|this_week|this_month|last_month"}
            | {"kind":"explicit","from":"DD.MM.YYYY","to":"DD.MM.YYYY"},
  "compare_to": {"kind":"relative","token":"prev_period"},   // только для class B
  "method": "plain" | "compare" | "contribution" | "top_n",
  "top_n": <int>,
  "filters": [ {"field":"<имя фильтра>","op":"in|eq|range","values":["<ИМЕНА, не uuid>"]} ],
  "output": {"format":"auto|text|xlsx"},
  "confidence": <0..1>
}

Правила:
- Если период не указан явно — всё равно выбери разумный токен и снизь confidence.
- В фильтрах указывай ИМЕНА точек/сотрудников/товаров, не идентификаторы.
- Никаких полей и фильтров вне white-list. Никакого текста вне JSON.`
}

// parsePlan извлекает JSON-план из ответа модели (на случай обрамления текстом).
func parsePlan(raw string) (plan.AnalysisPlan, error) {
	s := strings.TrimSpace(raw)
	if i := strings.IndexByte(s, '{'); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndexByte(s, '}'); j >= 0 {
		s = s[:j+1]
	}
	var p plan.AnalysisPlan
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return plan.AnalysisPlan{}, fmt.Errorf("не удалось распарсить план из ответа модели: %w", err)
	}
	if p.Version == "" {
		p.Version = "1"
	}
	return p, nil
}
