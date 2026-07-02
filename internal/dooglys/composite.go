package dooglys

import (
	"context"
	"errors"
	"fmt"
)

// ErrReportNotLive — отчёт не подключён к живому источнику, а fixture-fallback
// выключен (prod): честный отказ вместо фикстурных чисел под видом настоящих.
var ErrReportNotLive = errors.New("dooglys: отчёт не подключён к живому источнику")

// CompositeClient маршрутизирует отчёты по разным источникам данных.
// byReport — точечные живые источники (Report-API/JSON API); fallback — фикстуры
// для всего прочего (dev/CI). В prod fallback = nil: см. ErrReportNotLive.
type CompositeClient struct {
	byReport map[string]Client // slug отчёта → источник
	fallback Client            // источник для всех прочих отчётов; nil → отказ
}

// NewComposite создаёт маршрутизатор: byReport — точечные источники по отчётам,
// fallback — для всего остального (nil → не подключённый отчёт получает отказ).
func NewComposite(byReport map[string]Client, fallback Client) *CompositeClient {
	return &CompositeClient{byReport: byReport, fallback: fallback}
}

// Fetch выбирает источник по slug отчёта и делегирует ему запрос.
func (c *CompositeClient) Fetch(ctx context.Context, q Query) (Result, error) {
	if cl, ok := c.byReport[q.Report]; ok {
		return cl.Fetch(ctx, q)
	}
	if c.fallback == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrReportNotLive, q.Report)
	}
	return c.fallback.Fetch(ctx, q)
}
