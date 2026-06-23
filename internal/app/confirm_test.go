package app

import (
	"strings"
	"testing"

	"dgsbot/internal/catalog"
	"dgsbot/internal/plan"
)

func TestIsAffirmation(t *testing.T) {
	yes := []string{"да", "Да", "да!", "да, верно", "ага", "угу", "ок", "Окей",
		"ok", "yes", "верно", "точно", "правильно", "подтверждаю", "всё верно",
		"все верно", "так точно", "ага давай", "да давай"}
	for _, s := range yes {
		if !isAffirmation(s) {
			t.Errorf("isAffirmation(%q) = false, want true", s)
		}
	}
	no := []string{"", "нет", "не уверен", "да покажи товары", "покажи выручку",
		"а за прошлый месяц", "нет, товары", "выручка за неделю"}
	for _, s := range no {
		if isAffirmation(s) {
			t.Errorf("isAffirmation(%q) = true, want false", s)
		}
	}
}

func TestPeriodPhrase(t *testing.T) {
	cases := map[string]plan.Period{
		"этот месяц":                        {Kind: "relative", Token: "this_month"},
		"последние 7 дней":                  {Kind: "relative", Token: "last_7_days"},
		"вчера":                             {Kind: "relative", Token: "yesterday"},
		"период с 01.06.2026 по 07.06.2026": {Kind: "explicit", From: "01.06.2026", To: "07.06.2026"},
	}
	for want, p := range cases {
		if got := periodPhrase(p); got != want {
			t.Errorf("periodPhrase(%+v) = %q, want %q", p, got, want)
		}
	}
	// Неизвестный токен показываем как есть (лучше, чем молчать).
	if got := periodPhrase(plan.Period{Kind: "relative", Token: "last_quarter"}); got != "last_quarter" {
		t.Errorf("неизвестный токен: got %q", got)
	}
}

func TestDescribePlanAndConfirmPrompt(t *testing.T) {
	c := catalog.Default()
	p := plan.AnalysisPlan{
		Report: "payment", Method: "plain",
		Period:  plan.Period{Kind: "relative", Token: "this_month"},
		Filters: []plan.Filter{{Field: "sale_point", Op: "in", Values: []string{"Выкса"}}},
	}
	got := describePlan(p, c)
	for _, want := range []string{"Выручка", "этот месяц", "точка: Выкса"} {
		if !strings.Contains(got, want) {
			t.Errorf("describePlan = %q, не содержит %q", got, want)
		}
	}
	cp := confirmPrompt(p, c)
	if !strings.Contains(cp, "Правильно понимаю") || !strings.Contains(cp, "«да»") {
		t.Errorf("confirmPrompt = %q", cp)
	}

	// top_n asc → антирейтинг; desc → топ.
	asc := describePlan(plan.AnalysisPlan{Report: "products", Method: "top_n", Order: "asc",
		Period: plan.Period{Kind: "relative", Token: "last_7_days"}}, c)
	if !strings.Contains(asc, "антирейтинг") {
		t.Errorf("ожидался антирейтинг: %q", asc)
	}
}
