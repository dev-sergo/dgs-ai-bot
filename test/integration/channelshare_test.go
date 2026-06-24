package integration

import (
	"context"
	"strings"
	"testing"
)

// «доля безналичных за месяц» проходит весь путь: stub→Refine(channel_share)→engine.
// Раньше уходило в contribution с путаным нарративом «доли изменения».
func TestChannelShareEndToEnd(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "доля безналичных за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope: %+v / %q", ans.Validation, ans.Text)
	}
	if ans.Envelope.Type != "payment_channel_share" {
		t.Fatalf("type = %s, want payment_channel_share", ans.Envelope.Type)
	}
	if ans.Envelope.Meta["method"] != "channel_share" {
		t.Errorf("method = %v, want channel_share", ans.Envelope.Meta["method"])
	}
	// Нарратив — про долю безналичных в процентах, не про «изменение».
	if !strings.Contains(ans.Text, "Безналичные за период") || !strings.Contains(ans.Text, "% выручки") {
		t.Errorf("нарратив не про долю безналичных:\n%s", ans.Text)
	}
	if strings.Contains(ans.Text, "Доля изменения") {
		t.Errorf("просочился нарратив contribution:\n%s", ans.Text)
	}
}
