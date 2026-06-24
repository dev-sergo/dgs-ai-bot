package app

import (
	"testing"
	"time"

	"dgsbot/internal/plan"
	"dgsbot/internal/tenantctx"
)

// TestResolvePeriodNormalizesExplicitYear — модель пинит прошлый год на запрос месяца без
// года; resolvePeriod должен перенести его к актуальному. Если пользователь назвал год в
// реплике — оставить как есть. Проверяем wiring (regex по тексту + dates.NormalizeExplicitYear).
func TestResolvePeriodNormalizesExplicitYear(t *testing.T) {
	msk, _ := time.LoadLocation("Europe/Moscow")
	a := &App{Now: func() time.Time { return time.Date(2026, 6, 24, 10, 0, 0, 0, msk) }}
	ten := tenantctx.Tenant{Timezone: "Europe/Moscow"}

	cases := []struct {
		name     string
		period   plan.Period
		text     string
		wantFrom string
		wantTo   string
	}{
		{
			name:     "июнь без года → текущий",
			period:   plan.Period{Kind: "explicit", From: "01.06.2023", To: "30.06.2023"},
			text:     "выручка за июнь",
			wantFrom: "01.06.2026", wantTo: "30.06.2026",
		},
		{
			name:     "диапазон без года → текущий",
			period:   plan.Period{Kind: "explicit", From: "01.06.2023", To: "15.06.2023"},
			text:     "продажи с 1 по 15 июня",
			wantFrom: "01.06.2026", wantTo: "15.06.2026",
		},
		{
			name:     "год назван пользователем → не трогаем",
			period:   plan.Period{Kind: "explicit", From: "01.06.2024", To: "30.06.2024"},
			text:     "выручка за июнь 2024",
			wantFrom: "01.06.2024", wantTo: "30.06.2024",
		},
	}
	for _, c := range cases {
		from, to, err := a.resolvePeriod(c.period, ten, c.text)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if from != c.wantFrom || to != c.wantTo {
			t.Errorf("%s: %s..%s, want %s..%s", c.name, from, to, c.wantFrom, c.wantTo)
		}
	}
}
