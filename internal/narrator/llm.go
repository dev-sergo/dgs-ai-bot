package narrator

import (
	"context"
	"encoding/json"
	"log/slog"

	"dgsbot/internal/envelope"
	"dgsbot/internal/llm"
)

// LLM — нарратор поверх модели: получает УЖЕ посчитанные факты и формулирует кратко.
// Числа модели даём готовыми; при ошибке падаем на детерминированный Compose.
type LLM struct {
	cli   *llm.Client
	model string
	// Logger — наблюдаемость fallback'а на детерминированный Compose. nil → молча.
	// Без него срыв модели (ошибка/пустой ответ/нерусский вывод qwen) неотличим от
	// штатного нарратива: в логе исхода видно «answer», а не тихую деградацию.
	Logger *slog.Logger
}

// NewLLM создаёт LLM-нарратор.
func NewLLM(cli *llm.Client, model string) *LLM { return &LLM{cli: cli, model: model} }

func (n *LLM) Narrate(ctx context.Context, e envelope.Envelope) (string, error) {
	// metric_label — точное имя показателя из каталога. Без него модель угадывает
	// метрику из чисел (баг: выручку в contribution описывала как «средний чек»).
	metricLabel, _ := e.Meta["metric_label"].(string)
	// premise — направление, заложенное в вопрос ("down"/"up"). Если оно расходится с
	// числами, модель должна поправить посылку, а не подыграть ей (см. systemPrompt).
	premise, _ := e.Meta["premise_dir"].(string)
	facts, _ := json.Marshal(map[string]any{
		"currency": e.Currency,
		"period":   e.Period,
		"metric":   metricLabel,
		"premise":  premise,
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
		n.logFallback(err, out)
		return Compose(e), nil
	}
	return out, nil
}

// logFallback фиксирует причину деградации на Compose, чтобы тихий срыв модели был
// виден в логах (частоту легко агрегировать по reason). nil-логгер — no-op.
func (n *LLM) logFallback(err error, out string) {
	if n.Logger == nil {
		return
	}
	reason := "non_russian"
	switch {
	case err != nil:
		reason = "error"
	case out == "":
		reason = "empty"
	}
	attrs := []any{"reason", reason}
	if err != nil {
		attrs = append(attrs, "err", err.Error())
	}
	n.Logger.Warn("narrator.fallback", attrs...)
}

const systemPrompt = `Ты — аналитик кафе. На входе JSON с УЖЕ посчитанными числами (summary, rows).
Сформулируй краткое (1–3 предложения) объяснение на русском: что произошло с показателем и за счёт чего.
СТРОГО используй только числа из входных данных — НИЧЕГО не придумывай и не пересчитывай.
Показатель называй ТОЛЬКО так, как указано в поле "metric" входных данных (напр. «Выручка»).
Если "metric" задан — это и есть основная метрика; НЕ подменяй её другой («средний чек», «прибыль»
и т.п.), даже если так кажется по числам. Если "metric" пуст — пиши нейтрально «показатель».
Если за предыдущий период данных нет (value_prev = 0 или отсутствует), НЕ считай проценты роста и доли
(«+100%», «доля 120%» — бессмыслица): прямо напиши, что за предыдущий период данных не было и сравнение неполное.
Поле "premise" — направление, которое подразумевал вопрос: "down" (спрашивали про спад) или "up" (про рост).
Если "premise" задано, а числа показывают ОБРАТНОЕ (premise="down", но delta_abs > 0, или premise="up", но
delta_abs < 0) — НАЧНИ с явной поправки посылки («За период не падение, а рост …» / «… не рост, а снижение …»),
и только потом давай разбор. Не подыгрывай ложной посылке. Если "premise" пусто или совпадает с числами — игнорируй его.
Без вступлений и Markdown, только суть.`
