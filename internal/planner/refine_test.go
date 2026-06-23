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

// Консультационные запросы про ЭТО заведение → intent="advice"; generic-советы → не трогаем.
func TestRefineAdvice(t *testing.T) {
	advice := []string{
		"на чём я теряю деньги?",
		"какие товары убрать из меню?",
		"как поднять выручку за месяц",
		"что мне улучшить в продажах?",
		"дай совет что убрать из меню",
		"на чём можно сэкономить за прошлый месяц",
	}
	for _, q := range advice {
		p := plan.AnalysisPlan{Intent: "off_topic"} // модель часто кладёт сюда off_topic
		RefineAdvice(q, &p)
		if p.Intent != "advice" {
			t.Errorf("%q: intent=%q, want advice", q, p.Intent)
		}
	}

	// Generic-советы без привязки к данным остаются как есть (off_topic).
	notAdvice := []string{
		"дай совет по развитию бизнеса",
		"посоветуй как развивать бизнес в целом",
		"выручка за неделю",
		"топ товаров за месяц",
	}
	for _, q := range notAdvice {
		p := plan.AnalysisPlan{Intent: "off_topic"}
		RefineAdvice(q, &p)
		if p.Intent == "advice" {
			t.Errorf("%q: ошибочно помечен advice", q)
		}
	}
}

// payment + payment_type-фильтр (баг follow-up «а по карте?») → фильтр снят, выбрана
// колонка канала, group_by=date. Без этого валидатор бракует план в out_of_scope.
func TestRefinePaymentChannelFilter(t *testing.T) {
	cases := []struct {
		name, value, wantMetric string
	}{
		{"card→sum_card", "card", "sum_card"},
		{"карта→sum_card", "карта", "sum_card"},
		{"cash→sum_cash", "cash", "sum_cash"},
		{"наличными→sum_cash", "наличными", "sum_cash"},
		{"online→onlayn", "online", "onlayn"},
		{"sbp→sbp", "сбп", "sbp"},
	}
	for _, c := range cases {
		p := plan.AnalysisPlan{
			Report:  "payment",
			Method:  "plain",
			Filters: []plan.Filter{{Field: "payment_type", Op: "in", Values: []string{c.value}}},
		}
		RefinePaymentChannelFilter(&p)
		if len(p.Filters) != 0 {
			t.Errorf("%s: payment_type-фильтр не снят: %+v", c.name, p.Filters)
		}
		if len(p.Metrics) != 1 || p.Metrics[0] != c.wantMetric {
			t.Errorf("%s: metrics=%v, want [%s]", c.name, p.Metrics, c.wantMetric)
		}
		if len(p.GroupBy) != 1 || p.GroupBy[0] != "date" {
			t.Errorf("%s: group_by=%v, want [date]", c.name, p.GroupBy)
		}
	}
}

// На paycheck/orders payment_type легален — Refine его не трогает.
func TestRefinePaymentChannelFilter_SkipsNonPayment(t *testing.T) {
	p := plan.AnalysisPlan{
		Report:  "paycheck",
		Method:  "plain",
		Filters: []plan.Filter{{Field: "payment_type", Op: "in", Values: []string{"card"}}},
	}
	RefinePaymentChannelFilter(&p)
	if len(p.Filters) != 1 {
		t.Errorf("payment_type на paycheck должен сохраниться, got %+v", p.Filters)
	}
}

// При contribution/compare метрику не переопределяем — снимаем лишь невалидный фильтр.
func TestRefinePaymentChannelFilter_KeepsAnalyticMetric(t *testing.T) {
	p := plan.AnalysisPlan{
		Report:  "payment",
		Method:  "contribution",
		Metrics: []string{"sum_all"},
		Filters: []plan.Filter{{Field: "payment_type", Op: "in", Values: []string{"card"}}},
	}
	RefinePaymentChannelFilter(&p)
	if len(p.Filters) != 0 {
		t.Errorf("невалидный payment_type-фильтр должен быть снят, got %+v", p.Filters)
	}
	if len(p.Metrics) != 1 || p.Metrics[0] != "sum_all" {
		t.Errorf("метрика contribution не должна меняться: %v", p.Metrics)
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

// method="" + period задан → plain (follow-up «а по карте?»).
func TestRefineDefaultMethod_SetsPlain(t *testing.T) {
	p := plan.AnalysisPlan{
		Intent: "report", Report: "payment",
		Period: plan.Period{Kind: "relative", Token: "last_7_days"},
	}
	RefineDefaultMethod(&p)
	if p.Method != "plain" {
		t.Errorf("method=%q, ожидался plain", p.Method)
	}
}

// method="" + period пустой → не трогаем (clarify спросит период).
func TestRefineDefaultMethod_SkipsEmptyPeriod(t *testing.T) {
	p := plan.AnalysisPlan{Intent: "report", Report: "payment"}
	RefineDefaultMethod(&p)
	if p.Method != "" {
		t.Errorf("method не должен меняться при пустом периоде, got %q", p.Method)
	}
}

// method уже задан → не трогаем.
func TestRefineDefaultMethod_SkipsExistingMethod(t *testing.T) {
	p := plan.AnalysisPlan{
		Intent: "report", Report: "payment", Method: "compare",
		Period: plan.Period{Kind: "relative", Token: "last_7_days"},
	}
	RefineDefaultMethod(&p)
	if p.Method != "compare" {
		t.Errorf("метод не должен меняться, got %q", p.Method)
	}
}
