// Package narrator — формулирует текстовое объяснение по УЖЕ посчитанному результату.
// Числа берутся из envelope, не из модели. Template детерминирован; LLM добавляет
// естественную формулировку поверх тех же фактов.
package narrator

import (
	"context"

	"dgsbot/internal/envelope"
)

// Narrator формулирует объяснение результата.
type Narrator interface {
	Narrate(ctx context.Context, e envelope.Envelope) (string, error)
}
