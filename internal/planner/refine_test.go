package planner

import (
	"testing"

	"dgsbot/internal/plan"
)

func TestRefineTopNOrder(t *testing.T) {
	cases := []struct {
		query, want string
	}{
		{"что не покупают за месяц", "asc"},
		{"худшие товары за неделю", "asc"},
		{"что продаётся хуже всего", "asc"},
		{"топ товаров за месяц", "desc"},
		{"какой товар приносит больше всего выручки", "desc"},
		{"самые популярные блюда", "desc"},
	}
	for _, c := range cases {
		p := plan.AnalysisPlan{Method: "top_n"}
		RefineTopNOrder(c.query, &p)
		if p.Order != c.want {
			t.Errorf("%q: order=%q, want %q", c.query, p.Order, c.want)
		}
	}
}

// Не top_n — порядок не трогаем.
func TestRefineTopNOrder_SkipsNonTopN(t *testing.T) {
	p := plan.AnalysisPlan{Method: "plain"}
	RefineTopNOrder("худшие товары за неделю", &p)
	if p.Order != "" {
		t.Errorf("для plain order не должен меняться, got %q", p.Order)
	}
}

// «какой товар виноват» → products+contribution с ВАЛИДНЫМИ полями products
// (иначе остаётся sum_all от модели и план не проходит валидацию).
func TestRefineProductContribution(t *testing.T) {
	p := plan.AnalysisPlan{
		Intent: "report", Report: "payment", Metrics: []string{"sum_all"},
		Method: "compare", Period: plan.Period{Kind: "relative", Token: "last_30_days"},
	}
	RefineProductContribution("какой товар виноват в падении выручки за месяц", &p)
	if p.Report != "products" || p.Method != "contribution" {
		t.Fatalf("ожидался products+contribution, got %s/%s", p.Report, p.Method)
	}
	if len(p.Metrics) != 1 || p.Metrics[0] != "amount" {
		t.Errorf("metrics=%v, ожидался [amount]", p.Metrics)
	}
	if len(p.GroupBy) != 1 || p.GroupBy[0] != "name" {
		t.Errorf("group_by=%v, ожидался [name]", p.GroupBy)
	}
}

// Обычный рейтинг товаров под product-contribution НЕ попадает.
func TestRefineProductContribution_SkipsPlainRanking(t *testing.T) {
	p := plan.AnalysisPlan{Intent: "report", Report: "products", Method: "top_n"}
	RefineProductContribution("какой товар самый популярный за месяц", &p)
	if p.Method != "top_n" {
		t.Errorf("обычный рейтинг не должен стать contribution, got %s", p.Method)
	}
}

// Рейтинг по сотрудникам → честный отказ (off_topic с готовым текстом).
func TestRefineEmployeeRanking_Refuses(t *testing.T) {
	queries := []string{
		"топ продавцов за прошлый месяц",
		"лучшие официанты за неделю",
		"рейтинг кассиров",
		"какой оператор обработал больше всего чеков",
		"худшие сотрудники по выручке",
		"кто из официантов продал меньше всего",
		"самый эффективный продавец за месяц",
	}
	for _, q := range queries {
		p := plan.AnalysisPlan{Intent: "report", Report: "products", Method: "top_n"}
		RefineEmployeeRanking(q, &p)
		if p.Intent != "off_topic" {
			t.Errorf("%q: intent=%q, ожидался off_topic", q, p.Intent)
		}
		if p.Reply != EmployeeRankingReply {
			t.Errorf("%q: Reply не выставлен", q)
		}
	}
}

// Легальные запросы рейтинг-по-сотрудникам НЕ трогает: фильтр по имени сотрудника,
// рейтинг ТОВАРОВ, обычные отчёты со словом «сотрудник» без рейтинга.
func TestRefineEmployeeRanking_SkipsLegal(t *testing.T) {
	queries := []string{
		"чеки сотрудника Иванова за неделю",
		"проданные товары в чеках сотрудника за неделю",
		"топ товаров за месяц",
		"лучшие блюда за неделю",
		"выручка официанта Петрова за месяц",
		"средний чек за вчера",
	}
	for _, q := range queries {
		p := plan.AnalysisPlan{Intent: "report", Report: "products", Method: "top_n"}
		RefineEmployeeRanking(q, &p)
		if p.Intent == "off_topic" {
			t.Errorf("%q: ошибочно отклонён как off_topic", q)
		}
	}
}
