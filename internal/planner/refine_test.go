package planner

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"dgsbot/internal/plan"
)

// RefineAdvice не должен помечать advice НИ ОДИН кейс корпуса, который ждёт другой intent
// (report/help/smalltalk/off_topic). Детерминированный страж: расширение adviceRe не может
// тихо утянуть отчётный запрос в advice (иначе eval-host просядет на ровном месте).
// Читает рабочий корпус eval-host — coupling намеренный: правка регекса сразу проверяется
// против всех живых формулировок без рига.
func TestRefineAdvice_CorpusNoFalsePositive(t *testing.T) {
	f, err := os.Open("../../test/eval/prompts.jsonl")
	if err != nil {
		t.Skipf("корпус недоступен: %v", err)
	}
	defer f.Close()

	type corpusCase struct {
		Query  string `json:"query"`
		Expect struct {
			Intent string `json:"intent"`
		} `json:"expect"`
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c corpusCase
		if err := json.Unmarshal(line, &c); err != nil {
			t.Fatalf("битая строка %q: %v", string(line), err)
		}
		if c.Expect.Intent == "advice" {
			continue // эти и должны быть advice
		}
		p := plan.AnalysisPlan{}
		RefineAdvice(c.Query, &p)
		if p.Intent == "advice" {
			t.Errorf("%q (ждали intent=%q) ошибочно помечен advice", c.Query, c.Expect.Intent)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}

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
		"на чём я теряю",     // без «деньги» — раньше не ловилось из-за \b после кириллицы
		"на чём я теряю за май 2025",
		"какие товары убрать из меню?",
		"как поднять выручку за месяц",
		"что мне улучшить в продажах?",
		"дай совет что убрать из меню",
		"на чём можно сэкономить за прошлый месяц",
		"посоветуй что-нибудь по выручке за месяц", // бизнес-якорь у «посоветуй»
		// N1 — свободные формулировки совета, раньше падавшие в off_topic:
		"как поднять средний чек",
		"как увеличить прибыль за месяц",
		"как повысить оборот",
		"как нарастить выручку",
		"где у меня проблемы",
		"в чём у меня проблема с продажами",
		"какие у меня проблемы",
		"где слабые места",
		"что оптимизировать",
	}
	for _, q := range advice {
		p := plan.AnalysisPlan{Intent: "off_topic"} // модель часто кладёт сюда off_topic
		RefineAdvice(q, &p)
		if p.Intent != "advice" {
			t.Errorf("%q: intent=%q, want advice", q, p.Intent)
		}
	}

	// Generic/off-domain «советы» без бизнес-якоря остаются как есть (off_topic).
	notAdvice := []string{
		"дай совет по развитию бизнеса",
		"посоветуй как развивать бизнес в целом",
		"посоветуй рецепт пиццы", // «посоветуй» без бизнес-якоря — не advice
		"посоветуй хорошее кино",
		"выручка за неделю",
		"топ товаров за месяц",
		"как поднять настроение", // глагол роста без бизнес-объекта — не advice
		"как повысить квалификацию",
	}
	for _, q := range notAdvice {
		p := plan.AnalysisPlan{Intent: "off_topic"}
		RefineAdvice(q, &p)
		if p.Intent == "advice" {
			t.Errorf("%q: ошибочно помечен advice", q)
		}
	}
}

// Период для advice: если пользователь срок не назвал — выдуманное моделью окно чистим,
// чтобы advise-ветка спросила период (регрессия: «на чём я теряю» молча считалось за
// last_30_days). Если срок в тексте есть — период модели сохраняем.
func TestRefineAdvice_ClearsInventedPeriod(t *testing.T) {
	// Срок НЕ назван → period чистится (модель проставила last_30_days от балды).
	noPeriod := []string{
		"на чём я теряю",
		"что мне улучшить",
		"какие товары убрать из меню",
	}
	for _, q := range noPeriod {
		p := plan.AnalysisPlan{Intent: "off_topic",
			Period: plan.Period{Kind: "relative", Token: "last_30_days"}}
		RefineAdvice(q, &p)
		if p.Intent != "advice" {
			t.Fatalf("%q: intent=%q, want advice", q, p.Intent)
		}
		if p.Period.Token != "" || p.Period.From != "" {
			t.Errorf("%q: период не очищен: %+v", q, p.Period)
		}
	}

	// Срок назван (относительный/месяц/явные даты/год) → период модели сохраняем.
	withPeriod := []struct {
		q   string
		per plan.Period
	}{
		{"на чём я теряю за май 2025", plan.Period{Kind: "explicit", From: "2025-05-01", To: "2025-05-31"}},
		{"что улучшить за прошлый месяц", plan.Period{Kind: "relative", Token: "last_month"}},
		{"как поднять выручку за последние 7 дней", plan.Period{Kind: "relative", Token: "last_7_days"}},
		{"на чём теряю с 01.05 по 31.05", plan.Period{Kind: "explicit", From: "2025-05-01", To: "2025-05-31"}},
	}
	for _, tc := range withPeriod {
		p := plan.AnalysisPlan{Intent: "off_topic", Period: tc.per}
		RefineAdvice(tc.q, &p)
		if p.Intent != "advice" {
			t.Fatalf("%q: intent=%q, want advice", tc.q, p.Intent)
		}
		if p.Period != tc.per {
			t.Errorf("%q: период затёрт, был %+v стал %+v", tc.q, tc.per, p.Period)
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
		{"карточка→sum_card", "карточка", "sum_card"},
		{"cash→sum_cash", "cash", "sum_cash"},
		{"наличными→sum_cash", "наличными", "sum_cash"},
		{"наличка→sum_cash", "наличка", "sum_cash"},
		{"налик→sum_cash", "налик", "sum_cash"},
		{"online→onlayn", "online", "onlayn"},
		{"sbp→sbp", "сбп", "sbp"},
	}
	for _, c := range cases {
		p := plan.AnalysisPlan{
			Report:  "payment",
			Method:  "plain",
			Filters: []plan.Filter{{Field: "payment_type", Values: []string{c.value}}},
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

// «безнал» раскладывается в три колонки (card+online+sbp), не одну.
func TestRefinePaymentChannelFilter_Cashless(t *testing.T) {
	for _, v := range []string{"безнал", "безналом"} {
		p := plan.AnalysisPlan{
			Report:  "payment",
			Method:  "plain",
			Filters: []plan.Filter{{Field: "payment_type", Values: []string{v}}},
		}
		RefinePaymentChannelFilter(&p)
		if len(p.Filters) != 0 {
			t.Errorf("%s: payment_type-фильтр не снят: %+v", v, p.Filters)
		}
		want := []string{"sum_card", "onlayn", "sbp"}
		if len(p.Metrics) != len(want) {
			t.Fatalf("%s: metrics=%v, want %v", v, p.Metrics, want)
		}
		for i, m := range want {
			if p.Metrics[i] != m {
				t.Errorf("%s: metrics=%v, want %v", v, p.Metrics, want)
				break
			}
		}
	}
}

// На paycheck/orders payment_type легален — Refine его не трогает.
func TestRefinePaymentChannelFilter_SkipsNonPayment(t *testing.T) {
	p := plan.AnalysisPlan{
		Report:  "paycheck",
		Method:  "plain",
		Filters: []plan.Filter{{Field: "payment_type", Values: []string{"card"}}},
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
		Filters: []plan.Filter{{Field: "payment_type", Values: []string{"card"}}},
	}
	RefinePaymentChannelFilter(&p)
	if len(p.Filters) != 0 {
		t.Errorf("невалидный payment_type-фильтр должен быть снят, got %+v", p.Filters)
	}
	if len(p.Metrics) != 1 || p.Metrics[0] != "sum_all" {
		t.Errorf("метрика contribution не должна меняться: %v", p.Metrics)
	}
}

// «выручка сотрудника/товара/категории» → payment с фильтром, который payment не
// держит. Guard переводит на products (тоже несёт выручку amount), чистит payment-метрики.
func TestRefinePaymentEntityReport(t *testing.T) {
	for _, field := range []string{"user", "product", "product_category"} {
		p := plan.AnalysisPlan{
			Intent: "report", Report: "payment", Method: "plain",
			Metrics: []string{"sum_all"}, GroupBy: []string{"date"},
			Filters: []plan.Filter{{Field: field, Values: []string{"Иванов"}}},
		}
		RefinePaymentEntityReport(&p)
		if p.Report != "products" {
			t.Errorf("%s: report=%s, ожидался products", field, p.Report)
		}
		if len(p.Metrics) != 0 {
			t.Errorf("%s: payment-метрики не очищены: %v", field, p.Metrics)
		}
		if len(p.GroupBy) != 0 {
			t.Errorf("%s: payment group_by не очищен: %v", field, p.GroupBy)
		}
		// Фильтр со значением должен уцелеть — это и есть суть («не потеряли сотрудника»).
		if len(p.Filters) != 1 || p.Filters[0].Values[0] != "Иванов" {
			t.Errorf("%s: фильтр потерян: %+v", field, p.Filters)
		}
	}
}

// sale_point/locality payment держит сам → рероута быть не должно.
func TestRefinePaymentEntityReport_KeepsPaymentForOwnFilters(t *testing.T) {
	for _, field := range []string{"sale_point", "locality"} {
		p := plan.AnalysisPlan{
			Intent: "report", Report: "payment", Method: "plain",
			Filters: []plan.Filter{{Field: field, Values: []string{"Выкса"}}},
		}
		RefinePaymentEntityReport(&p)
		if p.Report != "payment" {
			t.Errorf("%s: payment не должен рероутиться, got %s", field, p.Report)
		}
	}
}

// Аналитику B-класса (compare/contribution) и не-payment отчёты guard не трогает.
func TestRefinePaymentEntityReport_SkipsAnalyticAndNonPayment(t *testing.T) {
	// contribution на payment с user-фильтром — method не plain, не трогаем.
	p := plan.AnalysisPlan{
		Intent: "report", Report: "payment", Method: "contribution",
		Filters: []plan.Filter{{Field: "user", Values: []string{"Иванов"}}},
	}
	RefinePaymentEntityReport(&p)
	if p.Report != "payment" {
		t.Errorf("contribution не должен рероутиться, got %s", p.Report)
	}
	// products c user-фильтром — уже верный отчёт, не трогаем.
	p2 := plan.AnalysisPlan{
		Intent: "report", Report: "products", Method: "plain",
		Filters: []plan.Filter{{Field: "user", Values: []string{"Иванов"}}},
	}
	RefinePaymentEntityReport(&p2)
	if p2.Report != "products" {
		t.Errorf("products должен остаться products, got %s", p2.Report)
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

// «доля безналичных / онлайн / по карте» → method=channel_share с нужным focus-каналом.
func TestRefineChannelShare(t *testing.T) {
	cases := []struct {
		query string
		focus []string
	}{
		{"доля безналичных за июнь", []string{"sum_card", "onlayn", "sbp"}},
		{"какой процент оплат прошёл по карте за неделю", []string{"sum_card"}},
		{"доля онлайн-оплат за месяц", []string{"onlayn"}},
		{"сколько процентов через сбп вчера", []string{"sbp"}},
		{"доля наличных за месяц", []string{"sum_cash"}},
		{"покажи долю каналов оплаты за неделю", nil}, // общая структура
	}
	for _, c := range cases {
		p := plan.AnalysisPlan{Intent: "report", Report: "payment", Method: "contribution"}
		RefineChannelShare(c.query, &p)
		if p.Method != "channel_share" {
			t.Errorf("%q: method=%q, want channel_share", c.query, p.Method)
			continue
		}
		if p.Report != "payment" {
			t.Errorf("%q: report=%q, want payment", c.query, p.Report)
		}
		if !equalStrings(p.Metrics, c.focus) {
			t.Errorf("%q: metrics=%v, want %v", c.query, p.Metrics, c.focus)
		}
	}
}

// Не канально-долевые запросы и advice/off_topic channel_share НЕ трогает.
func TestRefineChannelShare_Skips(t *testing.T) {
	// Триггер доли есть, но якоря канала нет — не наше.
	skip := []string{
		"доля скидок за месяц",
		"процент возвратов за неделю",
		"выручка по карте за месяц", // нет триггера доли
		"топ товаров за месяц",
	}
	for _, q := range skip {
		p := plan.AnalysisPlan{Intent: "report", Report: "payment", Method: "plain"}
		RefineChannelShare(q, &p)
		if p.Method == "channel_share" {
			t.Errorf("%q: ошибочно стал channel_share", q)
		}
	}

	// Совет «как поднять долю безнала» остаётся advice, а не фактический channel_share.
	adv := plan.AnalysisPlan{Intent: "advice"}
	RefineChannelShare("как увеличить долю безналичных", &adv)
	if adv.Method == "channel_share" || adv.Intent != "advice" {
		t.Errorf("advice не должен превращаться в channel_share: intent=%q method=%q", adv.Intent, adv.Method)
	}

	// Явный off_topic (отказ) channel_share не реанимирует.
	off := plan.AnalysisPlan{Intent: "off_topic"}
	RefineChannelShare("доля безналичных", &off)
	if off.Intent != "off_topic" {
		t.Errorf("off_topic не должен меняться, got %q", off.Intent)
	}
}

// PremiseDirection вытаскивает направление, заложенное в причинный вопрос: спад/рост/нейтрально.
// Спад имеет приоритет в смешанной фразе («возвраты выросли, выручка упала»).
func TestPremiseDirection(t *testing.T) {
	cases := []struct{ query, want string }{
		{"почему упала выручка за месяц", "down"},
		{"из-за чего снизился оборот", "down"},
		{"что стало причиной падения продаж", "down"},
		{"почему просели продажи", "down"},
		{"за счёт чего вырос оборот", "up"},
		{"почему выросла выручка за неделю", "up"},
		{"причина роста продаж", "up"},
		{"возвраты выросли, а выручка упала — почему", "down"}, // спад приоритетнее
		{"сравни выручку за два месяца", ""},                   // нейтрально
		{"выручка за неделю", ""},
	}
	for _, c := range cases {
		if got := PremiseDirection(c.query); got != c.want {
			t.Errorf("%q: PremiseDirection=%q, want %q", c.query, got, c.want)
		}
	}
}

// refineCorpusEntry — структура строки корпуса eval для corpus-стражей.
type refineCorpusEntry struct {
	Query  string `json:"query"`
	Expect struct {
		Intent string `json:"intent"`
		Method string `json:"method"`
		Report string `json:"report"`
	} `json:"expect"`
}

// loadRefineCorpus загружает корпус eval и пропускает тест, если файл недоступен.
func loadRefineCorpus(t *testing.T) []refineCorpusEntry {
	t.Helper()
	f, err := os.Open("../../test/eval/prompts.jsonl")
	if err != nil {
		t.Skipf("корпус недоступен: %v", err)
		return nil
	}
	defer f.Close()
	var out []refineCorpusEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e refineCorpusEntry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("битая строка %q: %v", string(line), err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Corpus-страж: RefineForecast не должен помечать forecast ни один кейс корпуса
// с явным non-forecast методом. Расширение forecastRe не может тихо утянуть
// отчётный/compare/contribution запрос в forecast.
func TestRefineForecast_CorpusNoFalsePositive(t *testing.T) {
	for _, e := range loadRefineCorpus(t) {
		if e.Expect.Method == "forecast" || e.Expect.Method == "" {
			continue // пропускаем forecast-кейсы и кейсы без явного ожидаемого метода
		}
		p := plan.AnalysisPlan{Intent: e.Expect.Intent}
		RefineForecast(e.Query, &p)
		if p.Method == "forecast" {
			t.Errorf("%q (ожидался method=%q) ошибочно стал forecast", e.Query, e.Expect.Method)
		}
	}
}

// Corpus-страж: RefineChannelShare не должен переводить в channel_share кейсы
// с явным non-channel_share методом. «Доля» в нефинансовом контексте не должна
// ломать compare/contribution/top_n-запросы.
func TestRefineChannelShare_CorpusNoFalsePositive(t *testing.T) {
	for _, e := range loadRefineCorpus(t) {
		if e.Expect.Method == "channel_share" || e.Expect.Method == "" {
			continue
		}
		p := plan.AnalysisPlan{Intent: e.Expect.Intent, Report: e.Expect.Report, Method: e.Expect.Method}
		RefineChannelShare(e.Query, &p)
		if p.Method == "channel_share" {
			t.Errorf("%q (ожидался method=%q) ошибочно стал channel_share", e.Query, e.Expect.Method)
		}
	}
}

// Corpus-страж: RefineEmployeeRanking не должен отклонять (off_topic) кейсы корпуса
// с ожидаемым intent=report/advice/help/smalltalk. Слова «сотрудник/продавец» в
// легальном контексте (фильтр по имени, отчёт без рейтинга) не должны давать off_topic.
func TestRefineEmployeeRanking_CorpusNoFalsePositive(t *testing.T) {
	for _, e := range loadRefineCorpus(t) {
		if e.Expect.Intent == "off_topic" || e.Expect.Intent == "" {
			continue
		}
		p := plan.AnalysisPlan{Intent: e.Expect.Intent}
		RefineEmployeeRanking(e.Query, &p)
		if p.Intent == "off_topic" {
			t.Errorf("%q (ожидался intent=%q) ошибочно стал off_topic", e.Query, e.Expect.Intent)
		}
	}
}

// Corpus-страж: RefineProductContribution не должен переводить в contribution кейсы
// корпуса с явным non-contribution методом. «Товар» + «вырос/упал» без «виноват/вклад»
// не должны ломать compare/top_n/plain-запросы.
func TestRefineProductContribution_CorpusNoFalsePositive(t *testing.T) {
	for _, e := range loadRefineCorpus(t) {
		if e.Expect.Method == "contribution" || e.Expect.Method == "" {
			continue
		}
		p := plan.AnalysisPlan{Intent: e.Expect.Intent, Method: e.Expect.Method}
		RefineProductContribution(e.Query, &p)
		if p.Method == "contribution" {
			t.Errorf("%q (ожидался method=%q) ошибочно стал contribution", e.Query, e.Expect.Method)
		}
	}
}

// Расширенный pos-корпус RefineProductContribution — формулировки «виноват/вклад/из-за»
// по товарам. Дополняет единственный ранее написанный тест.
func TestRefineProductContribution_Corpus(t *testing.T) {
	triggers := []string{
		"какой товар виноват в падении выручки за месяц",
		"из-за каких товаров упала выручка",
		"вклад товаров в рост продаж",
		"какой товар повлиял на снижение оборота",
		"из-за какого товара просела выручка",
	}
	for _, q := range triggers {
		p := plan.AnalysisPlan{Intent: "report", Report: "payment", Method: "compare",
			Metrics: []string{"sum_all"}}
		RefineProductContribution(q, &p)
		if p.Method != "contribution" || p.Report != "products" {
			t.Errorf("%q: method=%q report=%q, want contribution/products", q, p.Method, p.Report)
		}
	}
	// Легальные рейтинги/отчёты по товарам contribution НЕ получают.
	notTriggers := []string{
		"топ товаров за месяц",
		"какой товар самый популярный за неделю",
		"какой товар приносит больше всего выручки",
		"сравни продажи за два месяца",
		"какие товары продавались за прошлый месяц",
		"лучший товар по прибыли",
	}
	for _, q := range notTriggers {
		p := plan.AnalysisPlan{Intent: "report", Report: "products", Method: "top_n"}
		RefineProductContribution(q, &p)
		if p.Method == "contribution" {
			t.Errorf("%q: ошибочно стал contribution", q)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RefineForecast — прогнозные запросы → method=forecast, report=payment.
func TestRefineForecast_Triggers(t *testing.T) {
	triggers := []string{
		// --- исходные кейсы ---
		"прогноз выручки за текущий месяц",
		"прогноз оборота на месяц",
		"дойду ли я до плана?",
		"дойдём ли до выручки миллион?",
		"если ничего не менять, сколько выручки будет",
		"к концу месяца какая будет выручка",
		"сколько заработаю к концу периода",
		"ожидаемая выручка за месяц",
		"выручка к концу месяца",
		"сколько выйдет за месяц",
		// --- новые семейства (batch-лог) ---
		// «прогноз на конец» без revenue-якоря сразу за словом:
		"прогноз на конец месяца",
		// «предсказание» — синоним прогноза:
		"предсказание по выручке за июнь",
		// «при текущем темпе» / «если темп сохранится»:
		"при текущем темпе сколько выйдет",
		"куда мы выйдем при текущем темпе",
		"какой будет выручка если темп сохранится",
		// «план/цель + число» — вопрос о достижимости цели:
		"план 500 тысяч — выйдем?",
		"цель 1 миллион — дойдём?",
		"план 2 млн — реально?",
		"цель 300к — достижима?",
		"выйдем ли на план 800 тысяч",
		"сможем ли выполнить план в 1.2 млн",
		"хватит ли темпа до плана 600 тыс",
		// «к <дата> <месяц>» — явный горизонт:
		"что ожидается к 30 июня",
		// «к концу <название месяца>» + выручка:
		"как дела с выручкой к концу июня",
		// «дойти до» (основа дойт-) + сумма:
		"дойти до 1 миллиона реально?",
		// «дотянем + до + число»:
		"дотянем ли до 500 тыс к концу июня",
		// «выйти на + число»:
		"выйти на 700 тысяч — успеем?",
		"при текущем темпе выйдем на миллион?",
	}
	for _, q := range triggers {
		p := plan.AnalysisPlan{Intent: "report"}
		RefineForecast(q, &p)
		if p.Method != "forecast" {
			t.Errorf("%q: method=%q, want forecast", q, p.Method)
		}
		if p.Report != "payment" {
			t.Errorf("%q: report=%q, want payment", q, p.Report)
		}
	}
}

// Не-прогнозные формулировки guard НЕ трогает.
func TestRefineForecast_NoFalsePositives(t *testing.T) {
	notForecast := []string{
		"выручка за месяц",
		"выручка за вчера",
		"топ товаров за неделю",
		"как изменилась выручка",
		"доля безналичных",
		"прогноз погоды",              // «прогноз» без revenue-якоря
		"к концу смены успею",         // «к концу» без revenue-якоря и без месяца
		"как поднять выручку",         // совет, а не прогноз
		"план работы на неделю",       // «план» без числа после него
		"цель ясна",                   // «цель» без числа
		"при текущей ситуации",        // «при текущ» без «темп»
		"сравни выручку с прошлым месяцем", // compare, не forecast
	}
	for _, q := range notForecast {
		p := plan.AnalysisPlan{Intent: "report"}
		RefineForecast(q, &p)
		if p.Method == "forecast" {
			t.Errorf("%q: ошибочно помечен forecast", q)
		}
	}
}

// help/smalltalk/advice — guard не трогает; off_topic перехватывает (LLM-мисклассификация).
func TestRefineForecast_SkipsNonReport(t *testing.T) {
	for _, intent := range []string{"advice", "help", "smalltalk"} {
		p := plan.AnalysisPlan{Intent: intent}
		RefineForecast("прогноз выручки за месяц", &p)
		if p.Method == "forecast" {
			t.Errorf("intent=%q: не должен стать forecast", intent)
		}
	}
}

// off_topic от LLM — guard перехватывает и переводит в forecast.
func TestRefineForecast_OverridesOffTopic(t *testing.T) {
	p := plan.AnalysisPlan{Intent: "off_topic"}
	RefineForecast("дойду ли я до плана к концу месяца", &p)
	if p.Method != "forecast" {
		t.Errorf("off_topic прогнозный вопрос: method=%q, want forecast", p.Method)
	}
	if p.Intent != "report" {
		t.Errorf("off_topic прогнозный вопрос: intent=%q, want report", p.Intent)
	}
}

// this_month → this_month_full при forecast.
func TestRefineForecast_UpgradesThisMonth(t *testing.T) {
	p := plan.AnalysisPlan{
		Intent: "report",
		Period: plan.Period{Kind: "relative", Token: "this_month"},
	}
	RefineForecast("прогноз выручки за текущий месяц", &p)
	if p.Period.Token != "this_month_full" {
		t.Errorf("period.token=%q, want this_month_full", p.Period.Token)
	}
}

// Без периода → this_month_full по умолчанию.
func TestRefineForecast_DefaultPeriod(t *testing.T) {
	p := plan.AnalysisPlan{Intent: "report"}
	RefineForecast("если ничего не менять, сколько выручки", &p)
	if p.Period.Token != "this_month_full" || p.Period.Kind != "relative" {
		t.Errorf("period=%+v, want {Kind:relative Token:this_month_full}", p.Period)
	}
}

// Явный период при прогнозе — не трогаем (пользователь сам назвал даты).
func TestRefineForecast_KeepsExplicitPeriod(t *testing.T) {
	p := plan.AnalysisPlan{
		Intent: "report",
		Period: plan.Period{Kind: "explicit", From: "01.06.2026", To: "30.06.2026"},
	}
	RefineForecast("прогноз выручки за июнь", &p)
	if p.Method != "forecast" {
		t.Errorf("method=%q, want forecast", p.Method)
	}
	if p.Period.From != "01.06.2026" || p.Period.To != "30.06.2026" {
		t.Errorf("явный период изменён: %+v", p.Period)
	}
}
