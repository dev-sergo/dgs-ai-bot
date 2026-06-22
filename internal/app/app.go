// Package app — оркестратор пайплайна: planner → validate → dates → client → engine → render.
// Не зависит от HTTP, поэтому полностью покрывается интеграционными тестами.
package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dates"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/engine"
	"dgsbot/internal/envelope"
	"dgsbot/internal/narrator"
	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
	"dgsbot/internal/render"
	"dgsbot/internal/resolver"
	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
)

// minConfidence — порог уверенности планировщика. Если модель явно вернула
// confidence ниже порога, мы не угадываем отчёт, а переспрашиваем у пользователя.
// confidence == 0 трактуется как «не указана» (stub-планировщик) и гейт не срабатывает.
const minConfidence = 0.5

// lowConfidencePrompt — переспрос при низкой уверенности планировщика.
const lowConfidencePrompt = "Не уверен, что правильно понял запрос. " +
	"Уточните, какой отчёт нужен (Выручка, Товары, Чеки, Заказы) и за какой период."

// Answer — результат обработки запроса.
type Answer struct {
	TenantID   string                `json:"tenant_id"`
	Plan       plan.AnalysisPlan     `json:"plan"`
	Validation plan.ValidationResult `json:"validation"`
	Envelope   *envelope.Envelope    `json:"envelope,omitempty"`
	Text       string                `json:"answer,omitempty"`
}

// App — собранный пайплайн.
type App struct {
	planner  planner.Planner
	tenants  *tenantctx.Store
	client   dooglys.Client
	resolver *resolver.Store
	narrator narrator.Narrator
	sessions *session.Store
	cat      *catalog.Catalog

	// Now инъектируется для детерминированных тестов.
	Now func() time.Time
	// Logger — структурный лог исходов Ask (аудит/наблюдаемость). По умолчанию slog.Default().
	Logger *slog.Logger
}

// New собирает оркестратор.
func New(pl planner.Planner, tenants *tenantctx.Store, client dooglys.Client, res *resolver.Store, nar narrator.Narrator, sess *session.Store) *App {
	return &App{
		planner:  pl,
		tenants:  tenants,
		client:   client,
		resolver: res,
		narrator: nar,
		sessions: sess,
		cat:      catalog.Default(),
		Now:      time.Now,
		Logger:   slog.Default(),
	}
}

// Ask — основной вход: текст → ответ.
func (a *App) Ask(ctx context.Context, tenantID, sessionID, text string) (ans Answer, err error) {
	ans.TenantID = tenantID

	// Структурный лог исхода — один раз на любой ветке возврата (аудит/наблюдаемость).
	start := time.Now()
	defer func() { a.logAsk(tenantID, sessionID, ans, err, time.Since(start)) }()

	// История диалога — для follow-up и возобновления уточнений.
	p, err := a.planner.Plan(ctx, a.history(sessionID), text)
	if err != nil {
		return ans, err
	}
	// Детерминированная пост-обработка плана: маршрутизация «какой товар виноват»
	// → products+contribution и направление рейтинга (худшие/лучшие) для top_n —
	// то, что модель делает нестабильно.
	planner.Refine(text, &p)
	ans.Plan = p

	// Не-данные интенты (help/smalltalk/off_topic) — отвечаем словами, без отчётов.
	if !p.IsReport() {
		ans.Validation = plan.ValidationResult{OK: true}
		ans.Text = a.replyForIntent(p)
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Низкая уверенность планировщика — не угадываем отчёт, а переспрашиваем.
	// Иначе нераспознанный/метавопрос превратился бы в заглушку-отчёт.
	if p.Confidence > 0 && p.Confidence < minConfidence {
		ans.Validation = plan.ValidationResult{NeedClarify: true, ClarifyPrompt: lowConfidencePrompt}
		ans.Text = lowConfidencePrompt
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	ans.Validation = plan.Validate(&p, a.cat)
	if !ans.Validation.OK {
		// Невалидно или нужно уточнение — отдаём как есть (ветка clarify/refusal).
		if ans.Validation.NeedClarify {
			ans.Text = ans.Validation.ClarifyPrompt
		} else {
			// План вышел за white-list (поле/фильтр/разбивка вне каталога) —
			// честно говорим, что так не умеем, а НЕ возвращаем пустой ответ.
			ans.Text = a.outOfScopeMessage()
		}
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	rep, _ := a.cat.Report(p.Report)

	// Применимость метода: contribution возможен только для отчётов с раскладкой
	// на компоненты (напр. payment). Иначе понижаем до compare — суммарное изменение
	// без разбивки осмысленно для любого отчёта и честнее пустой раскладки.
	engine.NormalizeMethod(&p)

	// Резолв фильтров: имена → uuid (для ref). Нерезолвнутое имя → уточнение.
	filters, clarify := a.resolveFilters(rep, p.Filters)
	if clarify != "" {
		ans.Validation.OK = false
		ans.Validation.NeedClarify = true
		ans.Validation.ClarifyPrompt = clarify
		ans.Text = clarify
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Контекст тенанта (таймзона/валюта). Неизвестный → дефолт Москва/RUB.
	t, ok := a.tenants.Lookup(tenantID)
	if !ok {
		t = tenantctx.Tenant{Timezone: "Europe/Moscow", Currency: "RUB", CurrencyPrecision: 2}
	}

	// Резолв периода в абсолютные даты по таймзоне тенанта.
	from, to, err := a.resolvePeriod(p.Period, t)
	if err != nil {
		return ans, err
	}

	period := envelope.Period{From: from, To: to, TZ: t.Timezone}
	currency := currencyOr(t.Currency)
	resNow, err := a.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: from, To: to, Filters: filters})
	if err != nil {
		return ans, err
	}

	var env envelope.Envelope
	switch p.Method {
	case "compare", "contribution":
		prevR, err := a.comparePeriod(p, dates.Range{From: from, To: to})
		if err != nil {
			return ans, err
		}
		periodPrev := envelope.Period{From: prevR.From, To: prevR.To, TZ: t.Timezone}
		resPrev, err := a.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: prevR.From, To: prevR.To, Filters: filters})
		if err != nil {
			return ans, err
		}
		metric := primaryMetric(p, rep)
		if p.Method == "compare" {
			env = engine.Compare(rep, metric, resNow, resPrev, tenantID, currency, period, periodPrev)
		} else {
			env = engine.Contribution(rep, metric, resNow, resPrev, p.TopN, tenantID, currency, period, periodPrev)
		}
	case "top_n":
		// Рейтинг строк по метрике. Измерение по умолчанию — как у plain.
		if len(p.GroupBy) == 0 && rep.DefaultDim != "" {
			p.GroupBy = []string{rep.DefaultDim}
		}
		env = engine.TopN(p, rep, resNow, tenantID, currency, period)
	default:
		// Если модель не задала измерение — берём дефолтное из каталога (напр. date),
		// иначе таблица потеряет смысловую колонку (строки без подписи).
		if len(p.GroupBy) == 0 && rep.DefaultDim != "" {
			p.GroupBy = []string{rep.DefaultDim}
		}
		env = engine.Plain(p, rep, resNow, tenantID, currency, period)
	}

	if len(resNow.FiltersApplied) > 0 {
		env.Meta["filters_applied"] = resNow.FiltersApplied
	}
	if len(resNow.FiltersSkipped) > 0 {
		env.Meta["filters_skipped"] = resNow.FiltersSkipped
	}

	// Честная пустота: нет данных — отдаём прямой текстовый ответ БЕЗ envelope.
	// Вырожденный (нулевой/пустой) envelope не отдаём намеренно: иначе UI рисует
	// бесполезную таблицу нулей с заголовком-болванкой вместо честного «данных нет».
	if isEmptyResult(p.Method, env) {
		ans.Text = emptyResultMessage(env)
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Нарратив — только для аналитики (class B) и только когда есть что объяснять.
	if (p.Method == "compare" || p.Method == "contribution") && a.narrator != nil {
		if txt, nerr := a.narrator.Narrate(ctx, env); nerr == nil && txt != "" {
			env.Narrative = txt
		}
	}
	ans.Envelope = &env
	ans.Text = render.Text(env)
	a.remember(sessionID, text, ans.Text)
	return ans, nil
}

// logAsk пишет одну структурную строку об исходе запроса (аудит/наблюдаемость).
// Покрывает все ветки Ask через defer: интент, отчёт/метод/период, исход, латентность.
func (a *App) logAsk(tenantID, sessionID string, ans Answer, err error, dur time.Duration) {
	if a.Logger == nil {
		return
	}
	attrs := []any{
		"tenant", tenantID,
		"session", sessionID,
		"intent", ans.Plan.EffectiveIntent(),
		"outcome", askOutcome(ans, err),
		"latency_ms", dur.Milliseconds(),
	}
	if ans.Plan.IsReport() {
		attrs = append(attrs, "report", ans.Plan.Report, "method", ans.Plan.Method, "period", ans.Plan.Period.Token)
	}
	if err != nil {
		a.Logger.Error("ask", append(attrs, "err", err.Error())...)
		return
	}
	a.Logger.Info("ask", attrs...)
}

// askOutcome классифицирует исход для лога:
// error | off_topic | help | smalltalk | clarify | out_of_scope | empty | answer.
func askOutcome(ans Answer, err error) string {
	switch {
	case err != nil:
		return "error"
	case !ans.Plan.IsReport():
		return ans.Plan.EffectiveIntent() // off_topic | help | smalltalk
	case ans.Validation.NeedClarify:
		return "clarify"
	case !ans.Validation.OK:
		return "out_of_scope"
	case ans.Envelope != nil && len(ans.Envelope.Rows) == 0 &&
		ans.Plan.Method != "compare" && ans.Plan.Method != "contribution":
		return "empty"
	default:
		return "answer"
	}
}

// isEmptyResult сообщает, что результат вырожден (данных нет / разложить нечего).
// Для class B пустой Rows у Compare — норма, поэтому смотрим на суммы периодов.
func isEmptyResult(method string, e envelope.Envelope) bool {
	switch method {
	case "compare", "contribution":
		if e.Summary["value_now"] == 0 && e.Summary["value_prev"] == 0 {
			return true // в обоих периодах пусто — объяснять нечего
		}
		// Не удалось разложить метрику на компоненты (нет components у отчёта).
		return method == "contribution" && len(e.Rows) == 0
	default:
		return len(e.Rows) == 0
	}
}

// emptyResultMessage — честный ответ, когда данных для отчёта нет.
func emptyResultMessage(e envelope.Envelope) string {
	return "За период " + e.Period.From + " … " + e.Period.To +
		" данных для отчёта нет. Попробуйте другой период или уточните запрос."
}

// replyForIntent формирует текстовый ответ для не-отчётных интентов.
func (a *App) replyForIntent(p plan.AnalysisPlan) string {
	switch p.EffectiveIntent() {
	case "help":
		return a.helpText()
	case "smalltalk":
		if p.Reply != "" {
			return p.Reply
		}
		return "Здравствуйте! Я помогаю с аналитикой вашего заведения. " + a.helpHint()
	case "off_topic":
		return "Я отвечаю на вопросы по аналитике вашего заведения. " + a.helpHint()
	default:
		return a.helpText()
	}
}

// outOfScopeMessage — честный ответ, когда запрос вышел за white-list
// (запрошены поля/разбивки/фильтры, которых нет в каталоге). Не строим неверный
// отчёт и не молчим — объясняем границы и подсказываем, что доступно.
func (a *App) outOfScopeMessage() string {
	return "Такой разрез я пока не умею собирать — доступны только отчёты из каталога " +
		"и их стандартные разбивки. " + a.helpHint()
}

// helpText — список возможностей, собранный из каталога (не выдумывается моделью).
func (a *App) helpText() string {
	var names []string
	for _, slug := range a.cat.Slugs() {
		if r, ok := a.cat.Report(slug); ok {
			names = append(names, r.Name)
		}
	}
	return "Я — ассистент по аналитике вашего заведения. Могу показать отчёты: " +
		strings.Join(names, ", ") + ".\n" +
		"Также объясняю динамику — например «почему упала выручка за месяц».\n" +
		a.helpHint()
}

func (a *App) helpHint() string {
	return "Спросите, например: «выручка за неделю», «топ товаров за месяц», «сравни выручку с прошлой неделей»."
}

func (a *App) history(sessionID string) []session.Message {
	if a.sessions == nil {
		return nil
	}
	return a.sessions.History(sessionID)
}

func (a *App) remember(sessionID, userText, assistantText string) {
	if a.sessions != nil {
		a.sessions.Append(sessionID, userText, assistantText)
	}
}

// resolveFilters превращает фильтры плана в фильтры запроса.
// Для ref-фильтров имена резолвятся в uuid; первая неудача → текст уточнения.
func (a *App) resolveFilters(rep catalog.Report, pfs []plan.Filter) ([]dooglys.QueryFilter, string) {
	var out []dooglys.QueryFilter
	for _, pf := range pfs {
		cf, ok := rep.FilterByField(pf.Field)
		if !ok {
			continue // валидатор уже отсёк неизвестные; страхуемся
		}
		qf := dooglys.QueryFilter{Field: pf.Field, Param: cf.Param, Names: pf.Values}
		if cf.Kind == "ref" {
			for _, name := range pf.Values {
				m, err := a.resolver.Resolve(pf.Field, name)
				if err != nil {
					if re, ok := err.(*resolver.ResolveError); ok && re.Ambiguous {
						return nil, "Уточните " + pf.Field + " «" + name + "»: подходят " + joinRu(re.Candidates) + "."
					}
					return nil, "Не нашёл " + pf.Field + " «" + name + "». Проверьте название."
				}
				qf.UUIDs = append(qf.UUIDs, m.UUID)
			}
		}
		out = append(out, qf)
	}
	return out, ""
}

func joinRu(ss []string) string {
	return strings.Join(ss, ", ")
}

// comparePeriod определяет период сравнения: явный из плана или предыдущий равный.
func (a *App) comparePeriod(p plan.AnalysisPlan, cur dates.Range) (dates.Range, error) {
	if p.CompareTo != nil && p.CompareTo.Kind == "explicit" && p.CompareTo.From != "" {
		return dates.Range{From: p.CompareTo.From, To: p.CompareTo.To}, nil
	}
	return dates.PrevRange(cur)
}

// primaryMetric выбирает метрику для compare/contribution. Предпочитает денежное
// (RUB) поле отчёта: модель иногда ставит первым измерение (date/name), и наивный
// Metrics[0] дал бы суммирование «date» → 0 → ложное «данных нет».
func primaryMetric(p plan.AnalysisPlan, rep catalog.Report) string {
	for _, m := range p.Metrics {
		if f, ok := rep.FieldByKey(m); ok && f.Unit == "RUB" {
			return m
		}
	}
	if len(p.Metrics) > 0 {
		return p.Metrics[0]
	}
	return "sum_all"
}

func (a *App) resolvePeriod(p plan.Period, t tenantctx.Tenant) (from, to string, err error) {
	if p.Kind == "explicit" {
		return p.From, p.To, nil
	}
	r, err := dates.Resolve(p.Token, t.Location(), a.Now())
	if err != nil {
		return "", "", err
	}
	return r.From, r.To, nil
}

func currencyOr(c string) string {
	if c == "" {
		return "RUB"
	}
	return c
}
