package dooglys

import "context"

// CompositeClient маршрутизирует отчёты по разным источникам данных.
// На пилоте: payment — живой JSON API Dooglys, а products/paycheck/orders —
// локальные фикстуры (боевые товары/персонал ещё не подключены к API). Так
// консультант и «топ товаров» работают, не упираясь в неподдержанный отчёт.
type CompositeClient struct {
	byReport map[string]Client // slug отчёта → источник
	fallback Client            // источник для всех прочих отчётов
}

// NewComposite создаёт маршрутизатор: byReport — точечные источники по отчётам,
// fallback — для всего остального.
func NewComposite(byReport map[string]Client, fallback Client) *CompositeClient {
	return &CompositeClient{byReport: byReport, fallback: fallback}
}

// Fetch выбирает источник по slug отчёта и делегирует ему запрос.
func (c *CompositeClient) Fetch(ctx context.Context, q Query) (Result, error) {
	if cl, ok := c.byReport[q.Report]; ok {
		return cl.Fetch(ctx, q)
	}
	return c.fallback.Fetch(ctx, q)
}
