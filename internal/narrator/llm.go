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
	facts, _ := json.Marshal(map[string]any{
		"currency": e.Currency,
		"period":   e.Period,
		"summary":  e.Summary,
		"rows":     e.Rows,
	})
	msgs := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(facts)},
	}
	out, err := n.cli.Chat(ctx, n.model, msgs, llm.ChatOptions{Temperature: 0.2, MaxTokens: 300})
	if err != nil || out == "" {
		// Fallback: детерминированная формулировка из тех же чисел.
		return Compose(e), nil
	}
	return out, nil
}

const systemPrompt = `Ты — аналитик кафе. На входе JSON с УЖЕ посчитанными числами (summary, rows).
Сформулируй краткое (1–3 предложения) объяснение на русском: что произошло с показателем и за счёт чего.
СТРОГО используй только числа из входных данных — НИЧЕГО не придумывай и не пересчитывай.
Без вступлений и Markdown, только суть.`
