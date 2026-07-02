// Package app — оркестратор пайплайна: planner → validate → dates → client → engine → render.
// Не зависит от HTTP, поэтому полностью покрывается интеграционными тестами.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"dgsbot/internal/advisor"
	"dgsbot/internal/catalog"
	"dgsbot/internal/dates"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/engine"
	"dgsbot/internal/envelope"
	"dgsbot/internal/feedback"
	"dgsbot/internal/narrator"
	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
	"dgsbot/internal/querylog"
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

// unknownPeriodPrompt — переспрос когда модель выдала токен периода вне white-list.
// Не бросаем 500 — просто уточняем у пользователя конкретный период.
const unknownPeriodPrompt = "Не распознал период. Уточните: сегодня, вчера, " +
	"последние 7 или 30 дней, эта или прошлая неделя, этот или прошлый месяц."

// maxQueryRunes — предел длины одной реплики пользователя (в рунах). Легальный вопрос
// к аналитике — десятки слов; Telegram сам режет 4096 символов на сообщение, но HTTP
// /ask пропускает тело до 1 MiB — без этого предела «простыня» уходит прямо в промпт.
const maxQueryRunes = 2000

// queryTooLongPrompt — мягкий отказ на сверхдлинную реплику (не 500 и не ошибка LLM).
const queryTooLongPrompt = "Сообщение слишком длинное. Сформулируйте вопрос короче: " +
	"какой отчёт и за какой период вас интересует?"

// maintenancePrompt — ответ выключенного тенанта (kill-switch TENANT_<k>_ENABLED=0, R18).
const maintenancePrompt = "Бот временно на техобслуживании. Попробуйте позже."

// confirmConfidence — верхняя граница «полосы подтверждения». План с уверенностью
// в диапазоне [minConfidence, confirmConfidence) не исполняется сразу: бот эхом
// проговаривает интерпретацию и ждёт «да» (plan-confirm — дешёвый UX-шаг доверия).
// Ниже minConfidence — общий clarify (не угадываем вовсе), выше — исполняем сразу.
// Stub-планировщик отдаёт confidence==0 → полоса не срабатывает (детерминизм CI/eval).
const confirmConfidence = 0.7

// Answer — результат обработки запроса.
type Answer struct {
	ID         string                `json:"id"`
	TenantID   string                `json:"tenant_id"`
	Plan       plan.AnalysisPlan     `json:"plan"`
	Validation plan.ValidationResult `json:"validation"`
	Envelope   *envelope.Envelope    `json:"envelope,omitempty"`
	Text       string                `json:"answer,omitempty"`
}

// tenantSet — данные одного тенанта: источник (client) и резолвер имён→uuid его
// справочников. Это граница изоляции: запрос тенанта A обслуживается ТОЛЬКО его
// набором, никогда чужим. Резолвится по tenantID (см. setFor), а не хранится один на App.
type tenantSet struct {
	client   dooglys.Client
	resolver *resolver.Store
	uuid     string // Dooglys tenant_id (TENANT_<k>_ID); logged as tenant_id. Empty if unset.
}

// App — собранный пайплайн. Ядро (planner/narrator/advisor/sessions/tenants/catalog)
// тенант-агностично и общее; данные (client+resolver) — пер-тенантный реестр sets.
type App struct {
	planner planner.Planner
	tenants *tenantctx.Store
	// sets — реестр {client, resolver} по tenantID. Пополняется Register.
	sets map[string]*tenantSet
	// disabled — kill-switch по тенанту (TENANT_<k>_ENABLED=0): Ask отвечает
	// «на техобслуживании» без планирования и доступа к данным. Пополняется Disable.
	disabled map[string]bool
	// fallback — одно-тенантная деградация: набор, обслуживающий ЛЮБОЙ tenantID, если
	// он не зарегистрирован в sets. Ставится New (single-tenant/eval/HTTP-fixture); в
	// строгом мультитенантном режиме (NewMulti) nil → незарегистрированный тенант не
	// обслуживается (изоляция: не подставляем чужой источник по-умолчанию).
	fallback *tenantSet
	narrator narrator.Narrator
	advisor  advisor.Advisor
	sessions *session.Store
	cat      *catalog.Catalog

	// Now инъектируется для детерминированных тестов.
	Now func() time.Time
	// Logger — структурный лог исходов Ask (аудит/наблюдаемость). По умолчанию slog.Default().
	Logger *slog.Logger
	// QueryLog — дозапись датасета «вопрос → план → ответ» в JSONL (аналитика/дообучение).
	// nil → выключено. Включается в main по env QUERY_LOG_PATH.
	QueryLog *querylog.Writer
	// FeedbackLog — дозапись оценок пользователя (👍/👎) в JSONL.
	// nil → выключено. Включается по env FEEDBACK_LOG_PATH.
	FeedbackLog *feedback.Writer
}

// New собирает одно-тенантный оркестратор: переданный client+resolver обслуживает
// ЛЮБОЙ tenantID (fallback). Путь для eval/pipeval (корпус с разными tenant_id, один
// фикстурный источник), одно-тенантного dev и HTTP-фикстур. Мультитенант — NewMulti.
func New(pl planner.Planner, tenants *tenantctx.Store, client dooglys.Client, res *resolver.Store, nar narrator.Narrator, adv advisor.Advisor, sess *session.Store) *App {
	a := NewMulti(pl, tenants, nar, adv, sess)
	a.fallback = &tenantSet{client: client, resolver: res}
	return a
}

// NewMulti собирает мультитенантный оркестратор БЕЗ одно-тенантного fallback: источник
// данных обязателен пер-тенантно (Register). Незарегистрированный тенант не обслуживается
// — строгая изоляция (запрос тенанта A физически не может достать источник тенанта B).
func NewMulti(pl planner.Planner, tenants *tenantctx.Store, nar narrator.Narrator, adv advisor.Advisor, sess *session.Store) *App {
	return &App{
		planner:  pl,
		tenants:  tenants,
		sets:     map[string]*tenantSet{},
		narrator: nar,
		advisor:  adv,
		sessions: sess,
		cat:      catalog.Default(),
		Now:      time.Now,
		Logger:   slog.Default(),
	}
}

// Register binds a data source (client) and resolver to a routing tenantID, plus its
// Dooglys tenant_id UUID (tenantUUID; logged as tenant_id, may be empty). Called by
// bootstrap per tenant. Re-registering an existing tenantID is allowed (last wins).
func (a *App) Register(tenantID, tenantUUID string, client dooglys.Client, res *resolver.Store) {
	if a.sets == nil {
		a.sets = map[string]*tenantSet{}
	}
	a.sets[tenantID] = &tenantSet{client: client, resolver: res, uuid: tenantUUID}
}

// Disable выключает тенанта (kill-switch, TENANT_<k>_ENABLED=0): его запросы получают
// maintenancePrompt, остальные тенанты не затронуты. Включение обратно — правка env
// + рестарт (конфиг env-based, горячего reload нет).
func (a *App) Disable(tenantID string) {
	if a.disabled == nil {
		a.disabled = map[string]bool{}
	}
	a.disabled[tenantID] = true
}

// tenantUUID returns the Dooglys tenant_id UUID for a routing tenantID (empty if the
// tenant isn't registered or has no UUID set).
func (a *App) tenantUUID(tenantID string) string {
	if set := a.sets[tenantID]; set != nil {
		return set.uuid
	}
	return ""
}

// setFor возвращает набор данных тенанта. Строгая изоляция: сначала точная регистрация,
// затем одно-тенантный fallback (если задан). ok=false → тенант не обслуживается, и
// вызывающий обязан отказать, а НЕ подставить чужой источник.
func (a *App) setFor(tenantID string) (*tenantSet, bool) {
	if s, ok := a.sets[tenantID]; ok {
		return s, true
	}
	if a.fallback != nil {
		return a.fallback, true
	}
	return nil, false
}

// errUnknownTenant — тенант не зарегистрирован в реестре данных. Это инвариант
// конфигурации/изоляции (в проде боты шлют только свои зарегистрированные tenantID),
// поэтому возвращаем ошибку, а не молчаливую подстановку чужого источника.
var errUnknownTenant = errors.New("app: нет зарегистрированного источника данных для тенанта")

// fetchErrorPrompt — мягкий ответ при сбое обращения к источнику данных (живой API
// флакует/таймаутит). Реальная ошибка уходит в Logger, пользователю — человеческий текст,
// а НЕ raw error/500: и Telegram, и HTTP деградируют одинаково (см. dataUnavailable).
const fetchErrorPrompt = "Не смог получить данные — источник временно недоступен. Попробуйте позже."

// reportNotLivePrompt — отчёт не подключён к живому источнику (prod, ErrReportNotLive):
// это не транзиентный сбой, «попробуйте позже» ввёл бы в заблуждение.
const reportNotLivePrompt = "Этот отчёт пока не подключён. Доступны: выручка, товары, " +
	"категории, персонал, источники заказов и касса."

// dataUnavailable — единая точка graceful-деградации при сбое client.Fetch. Логирует
// реальную причину (аудит остаётся полным) и возвращает валидный Answer с мягким текстом
// и err=nil: сетевой сбой источника не должен всплывать пользователю как 500 или raw error.
// Отличается от errUnknownTenant (инвариант конфига → ошибка) — это транзиентный сбой данных.
func (a *App) dataUnavailable(sessionID, text string, ans Answer, cause error) (Answer, error) {
	if a.Logger != nil {
		a.Logger.Error("fetch.unavailable",
			"tenant", ans.TenantID, "report", ans.Plan.Report, "err", cause.Error())
	}
	ans.Validation = plan.ValidationResult{OK: false}
	ans.Text = fetchErrorPrompt
	if errors.Is(cause, dooglys.ErrReportNotLive) {
		ans.Text = reportNotLivePrompt
	}
	a.remember(sessionID, text, ans.Text)
	return ans, nil
}

// Ask — основной вход: текст → ответ.
func (a *App) Ask(ctx context.Context, tenantID, sessionID, text string) (ans Answer, err error) {
	ans.ID = newID()
	ans.TenantID = tenantID

	// Сверхдлинная реплика: HTTP пропускает тело до 1 MiB, а контекст модели конечен.
	// Отбиваем ДО планировщика и обрезаем text ДО лога/сессии, чтобы «простыня» не
	// раздувала ни промпт, ни JSONL-датасет, ни историю диалога.
	tooLong := utf8.RuneCountInString(text) > maxQueryRunes
	if tooLong {
		text = string([]rune(text)[:maxQueryRunes]) + "…"
	}

	// Структурный лог исхода — один раз на любой ветке возврата (аудит/наблюдаемость).
	start := time.Now()
	defer func() { a.logAsk(tenantID, sessionID, text, ans, err, time.Since(start)) }()

	if tooLong {
		ans.Text = queryTooLongPrompt
		return ans, nil
	}

	// Kill-switch тенанта (TENANT_<k>_ENABLED=0): мягкий отказ без планирования,
	// данных и записи в сессию; остальные тенанты работают как обычно.
	if a.disabled[tenantID] {
		ans.Text = maintenancePrompt
		return ans, nil
	}

	// Plan-confirm: если прошлой репликой мы переспросили «правильно понимаю …?»,
	// короткое «да» исполняет сохранённый план без повторного планирования. Pending
	// одноразовый — забираем его в начале хода при ЛЮБОМ ответе; не-«да» сбрасывает
	// его и идёт обычным путём (новая реплика планируется заново, не исполняя устаревшее).
	if pend, ok := a.takePending(sessionID); ok && isAffirmation(text) {
		if v := plan.Validate(&pend, a.cat); v.OK {
			return a.executeReport(ctx, tenantID, sessionID, text, pend)
		}
		// План внезапно невалиден (напр. изменился каталог) — обычный путь ниже.
	}

	// История диалога — для follow-up и возобновления уточнений.
	p, err := a.planner.Plan(ctx, a.history(sessionID), text)
	if err != nil {
		return ans, err
	}
	// Детерминированная пост-обработка плана: маршрутизация «какой товар виноват»
	// → products+contribution и направление рейтинга (худшие/лучшие) для top_n —
	// то, что модель делает нестабильно.
	planBefore := p // value-копия плана ДО refine — для трассировки шва
	planner.Refine(text, &p)
	a.logRefineDiff(text, planBefore, p)

	// Возобновление консультации: на прошлом ходу для разбора спросили период.
	// Если новая реплика несёт период — продолжаем advice (планировщик на голом
	// периоде теряет intent=advice и иначе выдал бы обычный отчёт). Single-shot.
	if pend, ok := a.takeAwaitingPeriod(sessionID); ok && hasPeriod(p.Period) {
		pend.Period = p.Period
		if len(p.Filters) > 0 {
			pend.Filters = p.Filters // перенесём уточнённые фильтры, если появились
		}
		p = pend
	}

	// Follow-up carry-over: для report-реплик без периода переносим период и фильтры
	// из последнего успешно исполненного плана. Триггер — отсутствие собственного периода
	// (сигнал уточняющей реплики). Фильтры мержатся по полю: поля из last.Filters, которых
	// нет в текущем плане, дополняют его (sale_point=Выкса переживает «а по карте?»).
	if p.IsReport() {
		if last, ok := a.lastPlan(sessionID); ok {
			noOwnPeriod := !hasPeriod(p.Period)
			if noOwnPeriod {
				p.Period = last.Period
			}
			if p.Report == "" {
				p.Report = last.Report
			}
			if noOwnPeriod && len(last.Filters) > 0 {
				byField := make(map[string]bool, len(p.Filters))
				for _, f := range p.Filters {
					byField[f.Field] = true
				}
				for _, f := range last.Filters {
					if !byField[f.Field] {
						p.Filters = append(p.Filters, f)
					}
				}
			}
		}
	}
	ans.Plan = p

	// Консультационный запрос («на чём теряю», «что улучшить») — отдельный режим:
	// детерминированный снимок бизнеса (несколько выборок) + advisor поверх чисел.
	if p.Intent == "advice" {
		return a.advise(ctx, tenantID, sessionID, text, p)
	}

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

	// Средняя уверенность планировщика — подтверждаем интерпретацию перед исполнением.
	// План валиден и исполним, но модель не уверена: вместо тихой догадки проговариваем
	// её словами и ждём «да». Низкая уверенность (<minConfidence) уже отбита общим clarify
	// выше; полоса [minConfidence, confirmConfidence) — конкретная догадка под подтверждение.
	if p.Confidence > 0 && p.Confidence < confirmConfidence {
		a.setPending(sessionID, p)
		msg := confirmPrompt(p, a.cat)
		ans.Validation = plan.ValidationResult{NeedClarify: true, ClarifyPrompt: msg}
		ans.Text = msg
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	return a.executeReport(ctx, tenantID, sessionID, text, p)
}

// executeReport исполняет уже провалидированный план отчёта: резолв фильтров/периода,
// выборка, движок, рендер. Вынесен из Ask, чтобы подтверждённый (plan-confirm) план
// шёл тем же путём, что и прямой запрос высокой уверенности.
func (a *App) executeReport(ctx context.Context, tenantID, sessionID, text string, p plan.AnalysisPlan) (Answer, error) {
	ans := Answer{TenantID: tenantID, Plan: p, Validation: plan.ValidationResult{OK: true}}

	// Данные тенанта резолвятся по tenantID (изоляция). Незарегистрированный тенант —
	// ошибка, а не чужой источник по-умолчанию.
	set, ok := a.setFor(tenantID)
	if !ok {
		if a.Logger != nil {
			a.Logger.Error("executeReport.unknown_tenant", "tenant", tenantID)
		}
		return ans, errUnknownTenant
	}

	// Отчёт обязан быть в каталоге: и прямой путь, и plan-confirm валидируют план до сюда.
	// Если каталог разъехался с планом — честный отказ и сигнал в лог, а НЕ молчаливый
	// пустой отчёт (rep=zero → все фильтры отброшены, Fetch по пустому отчёту).
	rep, ok := a.cat.Report(p.Report)
	if !ok {
		if a.Logger != nil {
			a.Logger.Warn("executeReport.unknown_report", "report", p.Report)
		}
		ans.Validation = plan.ValidationResult{OK: false}
		ans.Text = a.outOfScopeMessage()
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Применимость метода: contribution возможен только для отчётов с раскладкой
	// на компоненты (напр. payment). Иначе понижаем до compare — суммарное изменение
	// без разбивки осмысленно для любого отчёта и честнее пустой раскладки.
	engine.NormalizeMethod(&p)

	// Резолв фильтров: имена → uuid (для ref). Нерезолвнутое имя → уточнение.
	filters, clarify := a.resolveFilters(set.resolver, rep, p.Filters)
	if clarify != "" {
		ans.Validation.OK = false
		ans.Validation.NeedClarify = true
		ans.Validation.ClarifyPrompt = clarify
		ans.Text = clarify
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Контекст тенанта (таймзона/валюта). Неизвестный → дефолт Москва/RUB.
	t := a.tenant(tenantID)

	// Резолв периода в абсолютные даты по таймзоне тенанта.
	// Неизвестный токен — clarify, а не 500: модель иногда генерирует токены вне
	// white-list (last_14_days, last_week и т.п.) — лучше переспросить, чем крашиться.
	from, to, err := a.resolvePeriod(p.Period, t, text)
	if err != nil {
		var unknownTok *dates.ErrUnknownToken
		if errors.As(err, &unknownTok) {
			ans.Validation = plan.ValidationResult{NeedClarify: true, ClarifyPrompt: unknownPeriodPrompt}
			ans.Text = unknownPeriodPrompt
			a.remember(sessionID, text, ans.Text)
			return ans, nil
		}
		return ans, err
	}

	period := envelope.Period{From: from, To: to, TZ: t.Timezone}
	currency := currencyOr(t.Currency)
	resNow, err := set.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: from, To: to, Filters: filters})
	if err != nil {
		return a.dataUnavailable(sessionID, text, ans, err)
	}

	// Запрошенный фильтр построен, но отчёт его не поддерживает (нет такого разреза) —
	// честно говорим об этом, а НЕ показываем полный отчёт как будто это ответ на запрос.
	if len(resNow.FiltersSkipped) > 0 {
		ans.Validation = plan.ValidationResult{OK: false}
		ans.Text = a.skippedFilterMessage(rep, resNow.FiltersSkipped)
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	var env envelope.Envelope
	switch p.Method {
	case "compare", "contribution":
		prevR, err := a.comparePeriod(p, dates.Range{From: from, To: to})
		if err != nil {
			return ans, err
		}
		periodPrev := envelope.Period{From: prevR.From, To: prevR.To, TZ: t.Timezone}
		resPrev, err := set.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: prevR.From, To: prevR.To, Filters: filters})
		if err != nil {
			return a.dataUnavailable(sessionID, text, ans, err)
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
	case "channel_share":
		// Доля каналов оплаты за период (не сравнение): p.Metrics — выделенные каналы
		// (безнал/онлайн/карта), пусто → общая структура. Нарратив строит движок.
		env = engine.ChannelShare(resNow, p.Metrics, tenantID, currency, period)
	case "forecast":
		// Run-rate прогноз выручки поверх payment-ряда. asOf — «сегодня» в TZ тенанта;
		// from/to — полный целевой период (to = конец месяца, проставляется в A5).
		// resolvePeriod даёт DD.MM.YYYY; payment-ряд и RunRateForecast ждут ISO YYYY-MM-DD.
		loc := t.Location()
		asOf := a.Now().In(loc).Format("2006-01-02")
		fromISO, toISO := ruDateToISO(from), ruDateToISO(to)
		goal := extractGoal(text)
		fc := engine.RunRateForecast(resNow.Rows, fromISO, toISO, asOf)
		env = engine.ForecastEnvelope(fc, from, to, tenantID, currency, t.Timezone, goal)
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
		a.setLastPlan(sessionID, p)
		return ans, nil
	}

	// Нарратив — только для аналитики (class B) и только когда есть что объяснять.
	if (p.Method == "compare" || p.Method == "contribution") && a.narrator != nil {
		// Направление, заложенное в причинный вопрос («почему упала»). Нарратор поправит
		// посылку, если она расходится с числами, а не подыграет ей.
		if dir := planner.PremiseDirection(text); dir != "" {
			env.Meta["premise_dir"] = dir
		}
		if txt, nerr := a.narrator.Narrate(ctx, env); nerr == nil && txt != "" {
			env.Narrative = txt
		}
	}
	ans.Envelope = &env
	ans.Text = render.Text(env)
	a.remember(sessionID, text, ans.Text)
	a.setLastPlan(sessionID, p)
	return ans, nil
}

// advicePeriodPrompt — переспрос периода для консультации (совет без срока бессмыслен).
const advicePeriodPrompt = "За какой период подготовить разбор? " +
	"Например: последние 7 дней, последние 30 дней, этот или прошлый месяц."

// advise — режим консультанта: собирает детерминированный снимок бизнеса за период
// (выручка/возвраты/скидки/аутсайдеры из нескольких выборок) и формулирует совет через
// advisor поверх готовых чисел. Тонкий срез фокусируется на «на чём теряю / что улучшить».
func (a *App) advise(ctx context.Context, tenantID, sessionID, text string, p plan.AnalysisPlan) (Answer, error) {
	ans := Answer{TenantID: tenantID, Plan: p, Validation: plan.ValidationResult{OK: true}}

	// Данные тенанта по tenantID (изоляция). Незарегистрированный тенант — ошибка.
	set, ok := a.setFor(tenantID)
	if !ok {
		if a.Logger != nil {
			a.Logger.Error("advise.unknown_tenant", "tenant", tenantID)
		}
		return ans, errUnknownTenant
	}

	t := a.tenant(tenantID)

	// Период обязателен: пустой/неизвестный токен → уточняем, а не угадываем.
	from, to, err := a.resolvePeriod(p.Period, t, text)
	if err != nil {
		// Запоминаем advice-план: ответ-период на следующем ходу возобновит разбор,
		// а не выродится в обычный отчёт (планировщик теряет intent=advice на голом периоде).
		a.setAwaitingPeriod(sessionID, p)
		ans.Validation = plan.ValidationResult{NeedClarify: true, ClarifyPrompt: advicePeriodPrompt}
		ans.Text = advicePeriodPrompt
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}
	prev, err := dates.PrevRange(dates.Range{From: from, To: to})
	if err != nil {
		return ans, err
	}

	currency := currencyOr(t.Currency)
	period := envelope.Period{From: from, To: to, TZ: t.Timezone}
	periodPrev := envelope.Period{From: prev.From, To: prev.To, TZ: t.Timezone}

	// Фильтры плана (точка/категория) прокидываем в срез: каждый отчёт берёт только те
	// фильтры, что есть в его white-list (resolveFilters молча отсекает чужие), поэтому
	// «что улучшить на точке X» считает снимок по точке, а не по всему заведению.
	// Категория есть только у products, точка — у обоих.
	payRep, _ := a.cat.Report("payment")
	prodRep, _ := a.cat.Report("products")
	payFilters, clarify := a.resolveFilters(set.resolver, payRep, p.Filters)
	prodFilters, clarify2 := a.resolveFilters(set.resolver, prodRep, p.Filters)
	if clarify == "" {
		clarify = clarify2
	}
	if clarify != "" {
		ans.Validation = plan.ValidationResult{NeedClarify: true, ClarifyPrompt: clarify}
		ans.Text = clarify
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Снимок собирается из нескольких детерминированных выборок.
	payNow, err := set.client.Fetch(ctx, dooglys.Query{Report: "payment", From: from, To: to, Filters: payFilters})
	if err != nil {
		return a.dataUnavailable(sessionID, text, ans, err)
	}
	payPrev, err := set.client.Fetch(ctx, dooglys.Query{Report: "payment", From: prev.From, To: prev.To, Filters: payFilters})
	if err != nil {
		return a.dataUnavailable(sessionID, text, ans, err)
	}
	prodNow, err := set.client.Fetch(ctx, dooglys.Query{Report: "products", From: from, To: to, Filters: prodFilters})
	if err != nil {
		return a.dataUnavailable(sessionID, text, ans, err)
	}

	// Запрошенный фильтр построен, но отчёт его не поддерживает (нет такого разреза) —
	// честный отказ, а НЕ снимок по всему заведению под видом среза по точке/категории.
	if skipped := append(append([]string{}, payNow.FiltersSkipped...), prodNow.FiltersSkipped...); len(skipped) > 0 {
		rep := payRep
		if len(payNow.FiltersSkipped) == 0 {
			rep = prodRep
		}
		ans.Validation = plan.ValidationResult{OK: false}
		ans.Text = a.skippedFilterMessage(rep, dedupStrings(skipped))
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	bundle := engine.BuildInsightBundle(payNow, payPrev, prodNow, currency, period, periodPrev)

	// Нет данных за период — честный текст, без выдуманного совета.
	if bundle.Revenue.Now == 0 && len(bundle.BottomProducts) == 0 {
		ans.Text = "За период " + from + " … " + to + " данных для разбора нет. Попробуйте другой период."
		a.remember(sessionID, text, ans.Text)
		return ans, nil
	}

	// Совет: LLM-консультант поверх чисел снимка, с детерминированным fallback.
	txt := advisor.Compose(bundle)
	if a.advisor != nil {
		if out, aerr := a.advisor.Advise(ctx, bundle); aerr == nil && out != "" {
			txt = out
		}
	}
	ans.Text = txt
	a.remember(sessionID, text, ans.Text)
	return ans, nil
}

// logAsk пишет одну структурную строку об исходе запроса (аудит/наблюдаемость)
// и, если включён QueryLog, дозаписывает строку датасета (вопрос+план+ответ).
// Покрывает все ветки Ask через defer: интент, отчёт/метод/период, исход, латентность.
func (a *App) logAsk(tenantID, sessionID, text string, ans Answer, err error, dur time.Duration) {
	a.recordQuery(tenantID, sessionID, text, ans, err, dur)
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

// logRefineDiff трассирует детерминированную пост-обработку плана (refine): logAsk
// пишет только ФИНАЛьный план, поэтому «так сказала модель» и «так переписал refine»
// неразличимы. Логируем before/after ключевых полей (intent/report/method/metrics)
// и только когда что-то реально изменилось — иначе шум на каждый запрос.
func (a *App) logRefineDiff(text string, before, after plan.AnalysisPlan) {
	if a.Logger == nil {
		return
	}
	attrs := []any{"query", refineSnippet(text)}
	changed := false
	if before.EffectiveIntent() != after.EffectiveIntent() {
		changed = true
		attrs = append(attrs, "intent_before", before.EffectiveIntent(), "intent_after", after.EffectiveIntent())
	}
	if before.Report != after.Report {
		changed = true
		attrs = append(attrs, "report_before", before.Report, "report_after", after.Report)
	}
	if before.Method != after.Method {
		changed = true
		attrs = append(attrs, "method_before", before.Method, "method_after", after.Method)
	}
	if !sameStrings(before.Metrics, after.Metrics) {
		changed = true
		attrs = append(attrs, "metrics_before", strings.Join(before.Metrics, ","), "metrics_after", strings.Join(after.Metrics, ","))
	}
	if !changed {
		return
	}
	a.Logger.Info("planner.refine_changed", attrs...)
}

// refineSnippet обрезает текст запроса для лога (без раздувания строки).
func refineSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 120 {
		return s
	}
	return s[:120] + "…"
}

// sameStrings сравнивает срезы поэлементно (nil и [] эквивалентны).
func sameStrings(a, b []string) bool {
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

// recordQuery дозаписывает строку датасета «вопрос → план → ответ» для аналитики
// и дообучения. Пишется на любой ветке Ask; nil QueryLog (лог выключен) — no-op.
func (a *App) recordQuery(tenantID, sessionID, text string, ans Answer, err error, dur time.Duration) {
	if a.QueryLog == nil {
		return
	}
	rec := querylog.Record{
		TS:        a.Now().UTC().Format(time.RFC3339),
		ID:        ans.ID,
		Tenant:    tenantID,
		TenantID:  a.tenantUUID(tenantID),
		Session:   sessionID,
		Text:      text,
		Intent:    ans.Plan.EffectiveIntent(),
		Outcome:   askOutcome(ans, err),
		Plan:      ans.Plan,
		Answer:    ans.Text,
		LatencyMS: dur.Milliseconds(),
	}
	if ans.Envelope != nil {
		rec.Rows = len(ans.Envelope.Rows)
	}
	if err != nil {
		rec.Err = err.Error()
	}
	a.QueryLog.Write(rec)
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
	case "forecast":
		return false // нарратив всегда есть — строки пустые намеренно
	default:
		return len(e.Rows) == 0
	}
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

func (a *App) setLastPlan(sessionID string, p plan.AnalysisPlan) {
	if a.sessions != nil {
		a.sessions.SetLastPlan(sessionID, p)
	}
}

func (a *App) lastPlan(sessionID string) (plan.AnalysisPlan, bool) {
	if a.sessions == nil {
		return plan.AnalysisPlan{}, false
	}
	return a.sessions.LastPlan(sessionID)
}

// setPending запоминает план, ждущий подтверждения «да» (plan-confirm).
func (a *App) setPending(sessionID string, p plan.AnalysisPlan) {
	if a.sessions != nil {
		a.sessions.SetPending(sessionID, p)
	}
}

// takePending забирает (и удаляет) ожидающий подтверждения план. ok=false, если его нет.
func (a *App) takePending(sessionID string) (plan.AnalysisPlan, bool) {
	if a.sessions == nil {
		return plan.AnalysisPlan{}, false
	}
	return a.sessions.TakePending(sessionID)
}

// hasPeriod — есть ли в плане распознанный период (relative-токен или explicit-даты).
func hasPeriod(p plan.Period) bool {
	return p.Token != "" || p.From != ""
}

// setAwaitingPeriod запоминает advice-план, для которого спросили период (clarify-resume).
func (a *App) setAwaitingPeriod(sessionID string, p plan.AnalysisPlan) {
	if a.sessions != nil {
		a.sessions.SetAwaitingPeriod(sessionID, p)
	}
}

// takeAwaitingPeriod забирает advice-план, ждущий ответа про период. ok=false, если его нет.
func (a *App) takeAwaitingPeriod(sessionID string) (plan.AnalysisPlan, bool) {
	if a.sessions == nil {
		return plan.AnalysisPlan{}, false
	}
	return a.sessions.TakeAwaitingPeriod(sessionID)
}

// resolveFilters превращает фильтры плана в фильтры запроса.
// Для ref-фильтров имена резолвятся в uuid через резолвер тенанта (res); первая
// неудача → текст уточнения. res передаётся явно (пер-тенантный, из setFor).
func (a *App) resolveFilters(res *resolver.Store, rep catalog.Report, pfs []plan.Filter) ([]dooglys.QueryFilter, string) {
	var out []dooglys.QueryFilter
	for _, pf := range pfs {
		cf, ok := rep.FilterByField(pf.Field)
		if !ok {
			continue // валидатор уже отсёк неизвестные; страхуемся
		}
		qf := dooglys.QueryFilter{Field: pf.Field, Param: cf.Param, Names: pf.Values}
		if cf.Kind == "ref" {
			seen := make(map[string]bool)
			for _, name := range pf.Values {
				name = strings.TrimSpace(name)
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				m, err := res.Resolve(pf.Field, name)
				if err != nil {
					if re, ok := err.(*resolver.ResolveError); ok && re.Ambiguous {
						a.logResolverMiss(pf.Field, name, "ambiguous", re.Candidates)
						return nil, "Уточните " + pf.Field + " «" + name + "»: подходят " + joinRu(re.Candidates) + "."
					}
					a.logResolverMiss(pf.Field, name, "not_found", nil)
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

// logResolverMiss структурно логирует промах имя→uuid. В outcome-логе и not_found,
// и ambiguous слиты в outcome=clarify вместе с period/low-confidence clarify —
// неразличимо. Отдельная строка с kind/именем/типом промаха даёт точечный сигнал.
func (a *App) logResolverMiss(kind, name, missType string, candidates []string) {
	if a.Logger == nil {
		return
	}
	attrs := []any{"kind", kind, "name", name, "type", missType}
	if len(candidates) > 0 {
		attrs = append(attrs, "candidates", strings.Join(candidates, ","))
	}
	a.Logger.Warn("resolver.miss", attrs...)
}

// dedupStrings убирает повторы, сохраняя порядок (один фильтр пропущен в нескольких
// отчётах снимка → одно имя в сообщении об отказе, а не дубль).
func dedupStrings(ss []string) []string {
	seen := map[string]bool{}
	out := ss[:0:0]
	for _, s := range ss {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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

// yearInTextRe — пользователь сам назвал год в реплике (4 цифры 20xx). Тогда явный период
// не нормализуем: это осознанный выбор, а не угаданный моделью год.
var yearInTextRe = regexp.MustCompile(`20\d{2}`)

// rawText — исходная реплика: нужна, чтобы отличить «модель выдумала год» от «пользователь
// назвал год» при нормализации явного периода (см. dates.NormalizeExplicitYear).
func (a *App) resolvePeriod(p plan.Period, t tenantctx.Tenant, rawText string) (from, to string, err error) {
	if p.Kind == "explicit" {
		from, to = dates.NormalizeExplicitYear(
			p.From, p.To, yearInTextRe.MatchString(rawText), t.Location(), a.Now())
		return from, to, nil
	}
	r, err := dates.Resolve(p.Token, t.Location(), a.Now())
	if err != nil {
		return "", "", err
	}
	return r.From, r.To, nil
}

// tenant возвращает контекст тенанта (таймзона/валюта). Неизвестный идентификатор —
// дефолт Москва/RUB: один источник дефолта для executeReport и advise, чтобы они
// не разъехались. Timezone строкой — Location() лениво его парсит (см. tenantctx).
// После наложения env-конфига в bootstrap (tenantctx.Add) сюда попадать не должны —
// warn, чтобы «тихая Москва» у реального тенанта была видна в логе, а не в отчёте.
func (a *App) tenant(id string) tenantctx.Tenant {
	if t, ok := a.tenants.Lookup(id); ok {
		return t
	}
	if a.Logger != nil {
		a.Logger.Warn("tenant.ctx_missing", "tenant", id, "default", "Europe/Moscow, RUB")
	}
	return tenantctx.Tenant{Timezone: "Europe/Moscow", Currency: "RUB", CurrencyPrecision: 2}
}

func currencyOr(c string) string {
	if c == "" {
		return "RUB"
	}
	return c
}

// RecordFeedback записывает оценку пользователя в FeedbackLog.
// source — "ui" или "telegram". No-op если FeedbackLog выключен или id пустой.
func (a *App) RecordFeedback(ts, id, rating, source string) {
	if a.FeedbackLog == nil || id == "" {
		return
	}
	a.FeedbackLog.Write(feedback.Record{TS: ts, ID: id, Rating: rating, Source: source})
}

// newID генерирует короткий уникальный идентификатор ответа (12-char hex).
func newID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "000000000000"
	}
	return hex.EncodeToString(b)
}

// ruDateToISO конвертирует DD.MM.YYYY (формат Dooglys/resolvePeriod) в YYYY-MM-DD (ISO).
// RunRateForecast и payment-ряд используют ISO; resolvePeriod отдаёт DD.MM.YYYY.
func ruDateToISO(s string) string {
	t, err := time.Parse("02.01.2006", s)
	if err != nil {
		return s // не разобрали — возвращаем как есть; RunRateForecast вернёт no_data
	}
	return t.Format("2006-01-02")
}
