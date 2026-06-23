package dooglys

import (
	"context"
	"testing"
)

// fakeClient помечает результат своим тегом — чтобы видеть, какой источник отработал.
type fakeClient struct{ tag string }

func (f fakeClient) Fetch(_ context.Context, q Query) (Result, error) {
	return Result{Report: q.Report, Label: f.tag}, nil
}

func TestComposite_RoutesByReport(t *testing.T) {
	c := NewComposite(
		map[string]Client{"payment": fakeClient{"api"}},
		fakeClient{"fixture"},
	)

	pay, _ := c.Fetch(context.Background(), Query{Report: "payment"})
	if pay.Label != "api" {
		t.Errorf("payment должен идти в api, got %q", pay.Label)
	}

	for _, rep := range []string{"products", "paycheck", "orders"} {
		got, _ := c.Fetch(context.Background(), Query{Report: rep})
		if got.Label != "fixture" {
			t.Errorf("%s должен идти в fixture, got %q", rep, got.Label)
		}
	}
}
