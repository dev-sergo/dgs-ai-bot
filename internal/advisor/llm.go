package advisor

import (
	"context"
	"encoding/json"

	"dgsbot/internal/engine"
	"dgsbot/internal/llm"
)

// LLM — консультант поверх модели: получает УЖЕ посчитанный снимок (InsightBundle)
// и формулирует объяснение + рекомендации. При ошибке падает на детерминированный Compose.
type LLM struct {
	cli   *llm.Client
	model string
}

// NewLLM создаёт LLM-консультанта.
func NewLLM(cli *llm.Client, model string) *LLM { return &LLM{cli: cli, model: model} }

func (a *LLM) Advise(ctx context.Context, b engine.InsightBundle) (string, error) {
	facts, _ := json.Marshal(b)
	msgs := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(facts)},
	}
	out, err := a.cli.Chat(ctx, a.model, msgs, llm.ChatOptions{Temperature: 0.3, MaxTokens: 400})
	if err != nil || out == "" || hasNonRussian(out) {
		// Fallback: детерминированный совет из тех же чисел (в т.ч. если модель сорвалась
		// в другой язык — известная болячка qwen, см. roadmap 5.5).
		return Compose(b), nil
	}
	return out, nil
}

const systemPrompt = `Ты — бизнес-консультант кафе. На входе JSON со СНИМКОМ бизнеса за период —
УЖЕ посчитанные числа:
- revenue — выручка за период и динамика к прошлому периоду (delta_abs/delta_pct);
- avg_check — взвешенный средний чек (now/prev/delta_pct): средний размер чека и его динамика;
- channel_mix — каналы оплаты (карта/наличные/онлайн/СБП): сумма (now) и доля в выручке (share, %);
- returns_sum — деньги, ушедшие в возвраты; return_count — число возвратов;
- discounts — сумма выданных скидок;
- top_products — позиции-лидеры по выручке (на что опирается заведение);
- bottom_products — слабые позиции меню (выручка amount и прибыль profit).

Задача: кратко (3–5 предложений на русском) объяснить, на чём заведение теряет деньги или
что можно улучшить, и дать 1–3 КОНКРЕТНЫЕ рекомендации. Каждое утверждение подкрепляй
ЧИСЛОМ из снимка.

КАКИЕ ПОЛЯ КОГДА ПРИМЕНЯТЬ:
- «что улучшить / как поднять выручку» — опирайся на средний чек (avg_check: вырос или упал и на
  сколько) и каналы оплаты (channel_mix: какой канал преобладает, перекос долей), плюс возвраты и
  скидки как утечки. Совет про рост выручки без среднего чека или каналов — неполный.
- «на чём теряю деньги» — возвраты (returns_sum/return_count), скидки (discounts), убыточные
  позиции (profit<0 в bottom_products).
- «на что опираться / что работает» — top_products (лидеры по выручке).

СТРОГО:
- Используй ТОЛЬКО числа из входных данных — ничего не придумывай и не пересчитывай.
- Если revenue.empty_prev=true — не считай проценты роста/падения, скажи, что сравнить не с чем.
- Позиции с profit<0 в bottom_products — убыточные, прямо на это укажи.
- Отвечай ТОЛЬКО на русском языке. Без вступлений, без Markdown, только суть и рекомендации.`

// hasNonRussian грубо детектит срыв модели в другой алфавит (qwen иногда вставляет
// китайский, см. roadmap 5.5): если в ответе много не-латиницы/не-кириллицы — отбраковываем.
func hasNonRussian(s string) bool {
	var foreign, letters int
	for _, r := range s {
		switch {
		case r >= 'а' && r <= 'я', r >= 'А' && r <= 'Я', r == 'ё', r == 'Ё',
			r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letters++
		case r > 0x2000 && !isPunctOrSymbol(r):
			// За пределами базовой латиницы/кириллицы и не пунктуация/символы — чужой алфавит.
			foreign++
			letters++
		}
	}
	if letters == 0 {
		return false
	}
	return foreign*100/letters > 10 // >10% чужих букв → брак
}

func isPunctOrSymbol(r rune) bool {
	// Валюта/тире/типографские кавычки и т.п. — не считаем чужим алфавитом.
	switch r {
	case '₽', '—', '–', '…', '«', '»', '“', '”', '’', '№', '%':
		return true
	}
	return false
}
