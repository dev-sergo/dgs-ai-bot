package narrator

import (
	"context"
	"encoding/json"

	"dgsbot/internal/envelope"
	"dgsbot/internal/llm"
)

// LLM — нарратор поверх модели: получает УЖЕ посчитанные факты и формулирует кратко.
// Числа модели даём готовыми; при ошибке падаем на детерминированный Compose.
type LLM struct {
	cli   *llm.Client
	model string
}

// NewLLM создаёт LLM-нарратор.
func NewLLM(cli *llm.Client, model string) *LLM { return &LLM{cli: cli, model: model} }

func (n *LLM) Narrate(ctx context.Context, e envelope.Envelope) (string, error) {
	// metric_label — точное имя показателя из каталога. Без него модель угадывает
	// метрику из чисел (баг: выручку в contribution описывала как «средний чек»).
	metricLabel, _ := e.Meta["metric_label"].(string)
	facts, _ := json.Marshal(map[string]any{
		"currency": e.Currency,
		"period":   e.Period,
		"metric":   metricLabel,
		"summary":  e.Summary,
		"rows":     e.Rows,
	})
	msgs := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(facts)},
	}
	out, err := n.cli.Chat(ctx, n.model, msgs, llm.ChatOptions{Temperature: 0.2, MaxTokens: 300})
	if err != nil || out == "" || llm.HasNonRussian(out) {
		// Fallback: детерминированная формулировка из тех же чисел (в т.ч. если модель сорвалась
		// в другой язык — qwen иногда вставляет китайский в нарратив, см. roadmap 5.5).
		return Compose(e), nil
	}
	return out, nil
}

const systemPrompt = `Ты — аналитик кафе. На входе JSON с УЖЕ посчитанными числами (summary, rows).
Сформулируй краткое (1–3 предложения) объяснение на русском: что произошло с показателем и за счёт чего.
СТРОГО используй только числа из входных данных — НИЧЕГО не придумывай и не пересчитывай.
Показатель называй ТОЛЬКО так, как указано в поле "metric" входных данных (напр. «Выручка»).
Если "metric" задан — это и есть основная метрика; НЕ подменяй её другой («средний чек», «прибыль»
и т.п.), даже если так кажется по числам. Если "metric" пуст — пиши нейтрально «показатель».
Если за предыдущий период данных нет (value_prev = 0 или отсутствует), НЕ считай проценты роста и доли
(«+100%», «доля 120%» — бессмыслица): прямо напиши, что за предыдущий период данных не было и сравнение неполное.
Без вступлений и Markdown, только суть.`
