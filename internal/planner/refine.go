package planner

import (
	"regexp"
	"strings"

	"dgsbot/internal/plan"
)

// productContribRe ловит ЯВНЫЙ запрос раскладки изменения по товарам:
// «какой товар виноват в падении», «из-за каких товаров упала выручка», «вклад товаров».
// Узко и намеренно: фаззи-правило в промпте растекалось на top_n/compare-кейсы
// («что больше всего продали», «как изменились продажи») и давало регрессию,
// поэтому маршрутизируем детерминированно — как refusal-фильтр. Проверено: ни один
// кейс бенчмарка под этот паттерн не попадает (рейтинги «какой товар самый…» — нет
// слов виноват/вклад/из-за), поэтому общий pass-rate не затрагивается.
var productContribRe = regexp.MustCompile(
	`как(ой|ие)\s+товар\w*[^.?!]*(виноват|причин|из-за|повлия|вклад)` +
		`|вклад\s+товаров` +
		`|из-за\s+как(ого|их)\s+товар`)

// employeeRankWordRe / employeePersonRe — детектор РЕЙТИНГА по сотрудникам, которого
// каталог не поддерживает: измерения «сотрудник» нет, и модель молча подменяет такой
// запрос топом ТОВАРОВ (report=products) — выглядит уверенно, но это не то, что просили.
// Маршрутизируем детерминированно в честный отказ (как refusal-фильтр): нужен И слово
// рейтинга, И обозначение человека — порядок слов любой («топ продавцов» и «оператор
// обработал больше всего чеков»). Узко: легальный «чеки сотрудника Иванова» (фильтр по
// имени) под это не попадает — там нет слова рейтинга, плюс отдельная защита по имени.
var employeeRankWordRe = regexp.MustCompile(
	`топ|лучш|худш|рейтинг|больше\s+всего|меньше\s+всего|чаще\s+всего|реже\s+всего|эффективн|продуктивн|результативн|выработк`)
var employeePersonRe = regexp.MustCompile(
	`продавец|продавц|официант|кассир|сотрудник|оператор|персонал|бариста|менеджер`)

// employeeNameRe — фильтр по КОНКРЕТНОМУ сотруднику: «… сотрудника Иванова», «официант
// Петров». Имя — слово с заглавной кириллической буквы после обозначения человека.
// Такой запрос трогать нельзя: это легальная фильтрация, а не недоступный рейтинг.
// \w в Go RE2 = ASCII и кириллицу не матчит, поэтому хвосты слов задаём как [а-яё]*.
var employeeNameRe = regexp.MustCompile(
	`(продавц[а-яё]*|официант[а-яё]*|кассир[а-яё]*|сотрудник[а-яё]*|оператор[а-яё]*|бариста|менеджер[а-яё]*)\s+[А-ЯЁ][а-яё]+`)

// ascRe/descRe — направление рейтинга по смыслу запроса. Модель часто забывает
// выставить order на top_n («что не покупают» давало desc → показывало ЛУЧШИЕ).
// order не проверяется бенчмарком, поэтому фиксим детерминированно, без риска.
var ascRe = regexp.MustCompile(`не\s+покупа|не\s+бер[уёе]т|не\s+прода[ёе]тся|хуж|худш|меньше\s+всего|реже\s+всего|неходов|непопуляр|аутсайдер|антирейтинг|маленьк\w*\s+спрос|низк\w*\s+спрос|минимальн|наименьш`)
var descRe = regexp.MustCompile(`лучш|^топ|\sтоп|больше\s+всего|популярн|сам\w+\s+продава|хит\s+прода|ходов|доходн|прибыльн|чаще\s+всего|наибольш`)

// Refine — детерминированная пост-обработка плана (после планировщика): надёжно
// доводит то, что модель делает нестабильно. Применяется в app и eval (зеркало прода).
func Refine(query string, p *plan.AnalysisPlan) {
	RefineAdvice(query, p)
	RefineChannelShare(query, p)
	RefineEmployeeRanking(query, p)
	RefineProductContribution(query, p)
	RefinePaymentChannelFilter(p)
	RefineTopNOrder(query, p)
	RefineDefaultMethod(p)
}

// channelShareTriggerRe — запрос ДОЛИ/процента (а не суммы). Без канального якоря (ниже)
// не срабатывает: «доля скидок»/«процент возвратов» — другие метрики, не каналы оплаты.
var channelShareTriggerRe = regexp.MustCompile(`дол[яюие]|процент|сколько\s+процент|какая\s+част|в\s+процентах|удельн`)

// каналы оплаты для focus доли. Порядок проверки важен: «безнал» ловим РАНЬШЕ «нал».
var (
	beznalRe      = regexp.MustCompile(`безнал|без\s*нал|не\s*налич`)
	cashRe        = regexp.MustCompile(`налич|кэш|кеш`)
	cardRe        = regexp.MustCompile(`карт`)
	onlineRe      = regexp.MustCompile(`онлайн|online`)
	sbpRe         = regexp.MustCompile(`сбп|sbp|быстр[а-яё]*\s+платеж`)
	chanGenericRe = regexp.MustCompile(`канал[а-яё]*\s+оплат|способ[а-яё]*\s+оплат|структур[а-яё]*\s+оплат|дол[а-яё]*\s+канал`)
)

// channelShareFocus определяет, по какому каналу спрашивают долю. Второе значение —
// был ли вообще канальный якорь (без него RefineChannelShare не трогает план).
// Пустой срез при ok=true — общий вопрос о структуре оплат (показываем все каналы).
func channelShareFocus(q string) (focus []string, ok bool) {
	switch {
	case beznalRe.MatchString(q):
		return []string{"sum_card", "onlayn", "sbp"}, true
	case cashRe.MatchString(q):
		return []string{"sum_cash"}, true
	case cardRe.MatchString(q):
		return []string{"sum_card"}, true
	case onlineRe.MatchString(q):
		return []string{"onlayn"}, true
	case sbpRe.MatchString(q):
		return []string{"sbp"}, true
	case chanGenericRe.MatchString(q):
		return nil, true // структура всех каналов
	}
	return nil, false
}

// RefineChannelShare детерминированно маршрутизирует «доля безналичных / онлайн / по карте»
// в method=channel_share (доля канала за период) — раньше такой запрос уходил в contribution
// с путаным нарративом «доли изменения». Узкий guard: нужен И триггер доли, И канальный
// якорь. Не трогаем advice («как поднять долю безнала») и off_topic (явный отказ): channel_share
// — фактический отчёт, а не совет и не обход лимитов.
func RefineChannelShare(query string, p *plan.AnalysisPlan) {
	if p.Intent == "advice" || p.Intent == "off_topic" {
		return
	}
	q := strings.ToLower(query)
	if !channelShareTriggerRe.MatchString(q) {
		return
	}
	focus, ok := channelShareFocus(q)
	if !ok {
		return
	}
	p.Report = "payment"
	p.Method = "channel_share"
	p.Metrics = focus
	p.GroupBy = nil
	if p.Intent == "" {
		p.Intent = "report"
	}
}

// adviceRe — детектор КОНСУЛЬТАЦИОННЫХ запросов про ЭТО заведение: «на чём теряю»,
// «что убрать из меню», «как поднять выручку», «что улучшить», «дай совет». Это не
// фактологический отчёт, а просьба объяснить и порекомендовать → отдельный режим advice
// (снимок бизнеса + advisor-LLM). \w в Go RE2 кириллицу не матчит — хвосты как [а-яё]*.
var adviceRe = regexp.MustCompile(
	`на\s+ч[еёе]м[^.?!]*теря|теря[юеё][а-яё]*\s+деньг|где\s+(я\s+)?теря` +
		`|убрать\s+из\s+меню|как(ие|ой)?\s+товар[а-яё]*\s+убрать|что\s+убрать` +
		`|как\s+по\s*днять\s+(выручк|продаж|оборот|доход)|как\s+увеличить\s+(выручк|продаж|оборот|доход)` +
		`|что\s+(мне\s+)?(можно\s+)?улучшить|что\s+оптимизир|что\s+не\s+так\s+с\s+(продаж|выручк|бизнес)` +
		`|на\s+ч[еёе]м\s+(можно\s+)?с?эконом` +
		// «дай совет / посоветуй» — только с бизнес-якорем рядом, иначе ловит off-domain
		// («посоветуй рецепт пиццы», «посоветуй кино»).
		`|(дай\s+совет|посоветуй|что\s+посоветуешь)[^.?!]{0,30}(продаж|выручк|оборот|доход|меню|товар|скидк|возврат|прибыл|чек)`)

// adviceSkipRe — generic-советы БЕЗ привязки к данным заведения («совет по развитию
// бизнеса вообще») остаются off_topic: советовать «по бизнесу в целом» бот не должен.
var adviceSkipRe = regexp.MustCompile(
	`развити[а-яё]*\s+бизнес|бизнес[а-яё]*\s+(в\s+целом|вообще)|совет\s+по\s+(развити|бизнес)`)

// periodMentionRe — упоминание периода в СЫРОМ тексте: относительные слова (сегодня/
// неделя/месяц…), названия месяцев, явные даты (01.05) и годы (2025). Нужно, чтобы для
// advice не доверять выдуманному моделью периоду: на «на чём теряю» без срока LLM сама
// проставляет окно (last_30_days) и разбор молча считается на угаданном периоде. \w в Go
// RE2 кириллицу не матчит, \b после кириллицы не срабатывает — поэтому без них.
var periodMentionRe = regexp.MustCompile(
	`сегодня|вчера|позавчера|недел|месяц|квартал|полугод|год|сутк|посл[ае]дн|за\s+период` +
		`|январ|феврал|март|апрел|ма[йя]|июн|июл|август|сентябр|октябр|ноябр|декабр` +
		`|\d+\s*дн|\d{1,2}\.\d{1,2}|20\d{2}`)

// RefineAdvice детерминированно помечает консультационный запрос intent="advice" — узким
// guard'ом, НЕ фаззи-правилом в промпте (урок: фаззи растекается на соседей, роняет eval).
// Период оставляем ТОЛЬКО если пользователь назвал его в тексте: иначе модель выдумывает
// окно и разбор тихо считается не за тот срок. Нет упоминания → чистим, advise спросит.
func RefineAdvice(query string, p *plan.AnalysisPlan) {
	q := strings.ToLower(query)
	if adviceSkipRe.MatchString(q) || !adviceRe.MatchString(q) {
		return
	}
	p.Intent = "advice"
	p.Confidence = 1
	if !periodMentionRe.MatchString(q) {
		p.Period = plan.Period{} // срок не назван → не доверяем догадке модели
	}
}

// paymentChannelMetric маппит канал оплаты в КОЛОНКУ отчёта payment.
// В payment тип оплаты — это колонки (sum_card/sum_cash/onlayn/sbp), а не фильтр:
// фильтр payment_type есть только у paycheck/orders. В follow-up «а по карте?» модель
// иногда ставит payment_type-фильтр на payment → валидатор бракует план → ложный
// out_of_scope на легальный запрос. Принимаем и enum-значения (card/cash/online/sbp),
// и русские формы, которые модель порой кладёт в values.
var paymentChannelMetric = map[string]string{
	"card": "sum_card", "карта": "sum_card", "картой": "sum_card", "по карте": "sum_card",
	"cash": "sum_cash", "наличные": "sum_cash", "наличными": "sum_cash",
	"online": "onlayn", "онлайн": "onlayn",
	"sbp": "sbp", "сбп": "sbp", "по сбп": "sbp", "через сбп": "sbp",
}

// RefinePaymentChannelFilter снимает невалидный payment_type-фильтр с отчёта payment
// и переводит распознанный канал в выбор колонки. Без этого «а по карте?» (follow-up
// к «выручка за неделю») выпадает в out_of_scope. Применяется только к payment —
// у paycheck/orders payment_type легален и не трогается.
func RefinePaymentChannelFilter(p *plan.AnalysisPlan) {
	if p.Report != "payment" || len(p.Filters) == 0 {
		return
	}
	kept := p.Filters[:0]
	metric := ""
	for _, f := range p.Filters {
		if f.Field == "payment_type" {
			for _, v := range f.Values {
				if m, ok := paymentChannelMetric[strings.ToLower(strings.TrimSpace(v))]; ok {
					metric = m
					break
				}
			}
			continue // фильтр payment_type у payment невалиден — снимаем
		}
		kept = append(kept, f)
	}
	p.Filters = kept
	if metric == "" {
		return
	}
	// Канал распознан: для простого отчёта показываем его колонку. Для аналитики
	// (compare/contribution) метрику не трогаем — там канал раскладывает движок,
	// важно лишь снять невалидный фильтр.
	if p.Method == "" || p.Method == "plain" {
		p.Metrics = []string{metric}
		if len(p.GroupBy) == 0 {
			p.GroupBy = []string{"date"}
		}
	}
}

// RefineDefaultMethod выставляет method=plain когда модель вернула пустой method
// при уже заданном отчёте и периоде. Это происходит в follow-up запросах («а по карте?»):
// движок обработал бы method="" как plain через default-ветку, но валидатор сначала
// отбрасывает такой план как out_of_scope — пользователь получает «не умею» на
// легальный запрос. Если период не задан — не трогаем (модель сигнализирует
// неопределённость → clarify по периоду сработает нормально).
func RefineDefaultMethod(p *plan.AnalysisPlan) {
	if p.Method != "" {
		return
	}
	if p.Intent != "" && p.Intent != "report" {
		return
	}
	if p.Report == "" {
		return
	}
	// Период не задан → оставляем пустой method: clarify спросит период.
	if p.Period.Token == "" && (p.Period.Kind != "explicit" || p.Period.From == "") {
		return
	}
	p.Method = "plain"
}

// EmployeeRankingReply — честный отказ на рейтинг по сотрудникам. Объясняем границу
// (нет разреза по людям) и подсказываем, что доступно, включая фильтр по имени.
const EmployeeRankingReply = "Рейтинг по сотрудникам (продавцам, официантам, кассирам) " +
	"пока недоступен — в каталоге нет разреза по сотрудникам. Я могу показать выручку, " +
	"товары, чеки и заказы за период, а также чеки конкретного сотрудника по имени."

// RefineEmployeeRanking детерминированно переводит запрос рейтинга по сотрудникам
// («топ продавцов», «лучшие официанты», «какой оператор обработал больше всего чеков»)
// в честный отказ: помечает план off_topic с готовым текстом (app отдаёт его как ответ).
// Без этого модель выдаёт топ товаров — уверенно, но мимо запроса.
func RefineEmployeeRanking(query string, p *plan.AnalysisPlan) {
	if p.Intent != "" && p.Intent != "report" {
		return
	}
	q := strings.ToLower(query)
	if !employeeRankWordRe.MatchString(q) || !employeePersonRe.MatchString(q) {
		return
	}
	if employeeNameRe.MatchString(query) {
		return // «чеки сотрудника Иванова» — фильтр по имени, не рейтинг
	}
	p.Intent = "off_topic"
	p.Reply = EmployeeRankingReply
	p.Confidence = 1
}

// RefineTopNOrder выставляет направление рейтинга (asc — худшие, desc — лучшие)
// по смыслу запроса, если он однозначен. Только для top_n.
func RefineTopNOrder(query string, p *plan.AnalysisPlan) {
	if p.Method != "top_n" {
		return
	}
	q := strings.ToLower(query)
	asc, desc := ascRe.MatchString(q), descRe.MatchString(q)
	switch {
	case asc && !desc:
		p.Order = "asc"
	case desc && !asc:
		p.Order = "desc"
	}
}

// RefineProductContribution детерминированно фиксирует products+contribution для
// запросов «какой товар виноват в росте/падении» (раскладка изменения по товарам).
// Применяется ПОСЛЕ планировщика: модель такие формулировки путает с top_n/compare.
func RefineProductContribution(query string, p *plan.AnalysisPlan) {
	if p.Intent != "" && p.Intent != "report" {
		return
	}
	if !productContribRe.MatchString(strings.ToLower(query)) {
		return
	}
	p.Report = "products"
	p.Class = plan.ClassB
	p.Method = "contribution"
	// Приводим поля к валидным для products: иначе оставшиеся от модели метрики
	// payment (sum_all) не пройдут валидацию и ответ выродится в «не умею».
	p.Metrics = []string{"amount"}
	p.GroupBy = []string{"name"}
	if p.CompareTo == nil {
		p.CompareTo = &plan.Period{Kind: "relative", Token: "prev_period"}
	}
}
