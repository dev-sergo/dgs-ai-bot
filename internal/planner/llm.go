package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dgsbot/internal/catalog"
	"dgsbot/internal/llm"
	"dgsbot/internal/plan"
	"dgsbot/internal/session"
)

// LLMPlanner строит AnalysisPlan через модель с guided-JSON.
type LLMPlanner struct {
	cli       *llm.Client
	model     string
	cat       *catalog.Catalog
	forceJSON bool
}

// NewLLM создаёт планировщик поверх LLM-клиента.
func NewLLM(cli *llm.Client, model string, forceJSON bool) *LLMPlanner {
	return &LLMPlanner{cli: cli, model: model, cat: defaultCatalog(), forceJSON: forceJSON}
}

func (p *LLMPlanner) Plan(ctx context.Context, history []session.Message, query string) (plan.AnalysisPlan, error) {
	msgs := []llm.Message{{Role: "system", Content: p.systemPrompt()}}
	for _, h := range history {
		msgs = append(msgs, llm.Message{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: query})
	raw, err := p.cli.Chat(ctx, p.model, msgs, llm.ChatOptions{Temperature: 0, MaxTokens: 512, JSONObject: p.forceJSON})
	if err != nil {
		return plan.AnalysisPlan{}, err
	}
	pl, perr := parsePlan(raw)
	if perr != nil {
		// Показываем сырой ответ модели — это главный инструмент отладки реального вызова.
		return plan.AnalysisPlan{}, fmt.Errorf("%w | raw_model_output: %s", perr, snippet(raw, 500))
	}
	return pl, nil
}

func snippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (p *LLMPlanner) systemPrompt() string {
	return `Ты — ассистент по аналитике кафе. Сначала определи ТИП запроса (intent), затем верни строгий JSON.
Учитывай историю диалога: «а за прошлый месяц?», «а по Выксе?» — это уточнение предыдущего запроса.
Если в прошлой реплике ты просил уточнить период/параметр, новая реплика пользователя — это ответ на него.

intent:
- "report"    — пользователь хочет данные/отчёт → заполни поля плана ниже;
- "help"      — «что ты умеешь / какие функции» → верни {"intent":"help"};
- "smalltalk" — приветствие/благодарность/болтовня → {"intent":"smalltalk","reply":"<короткий дружелюбный ответ>"};
- "off_topic" — не про аналитику заведения → {"intent":"off_topic"}.
Для report поле "reply" не нужно.

Доступные отчёты и поля (white-list — использовать ТОЛЬКО их):
` + p.cat.Describe() + `
Для intent="report" верни JSON со схемой:
{
  "version": "1",
  "intent": "report",
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
- ВСЕГДА указывай "method" (для простого отчёта — "plain").
- ВСЕГДА заполняй "group_by": для payment — ["date"], для products — ["name"]; иначе таблица потеряет смысл.
- "period.token" НИКОГДА не оставляй пустым. Сопоставление фраз:
    сегодня→today; вчера→yesterday; за неделю/последнюю неделю→last_7_days;
    эта/текущая неделя→this_week; текущий/этот месяц→this_month;
    за месяц/последний месяц/последние 30 дней→last_30_days; прошлый месяц→last_month.
  Если период не назван — выбери this_month и снизь confidence.
- Выбор method для аналитики (class B):
    "почему", "за счёт чего", "что повлияло", "из-за чего" → "contribution" (раскладка вклада);
    "сравни", "насколько изменилось", "динамика", "относительно прошлого" → "compare".
  Для class B всегда задавай "compare_to": {"kind":"relative","token":"prev_period"}.
- "выручка картой/наличными" — это отчёт payment (колонки sum_card/sum_cash), фильтр не нужен.
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
