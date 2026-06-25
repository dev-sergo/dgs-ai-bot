package engine

import (
	"fmt"
	"time"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
	"dgsbot/internal/render"
)

// fcLayout — формат дат payment-ряда и границ периода (ISO, YYYY-MM-DD).
const fcLayout = "2006-01-02"

// forecastConfidentDays — порог «достаточно данных»: меньше полной недели закрытых
// суток → дисклеймер обязателен. Неделя покрывает недельный цикл (будни/выходные),
// который run-rate иначе не видит и систематически врёт на коротком окне.
const forecastConfidentDays = 7

// ForecastStatus — исход прогноза. Несёт дисклеймер вместе с LowConfidence: число
// без явной оговорки наверх не уходит (A2 — честность точности).
type ForecastStatus string

const (
	ForecastOK       ForecastStatus = "ok"        // спрогнозировано
	ForecastFact     ForecastStatus = "fact"      // период закрыт — это факт, не прогноз
	ForecastTooEarly ForecastStatus = "too_early" // нет ни одного полного дня
	ForecastNoData   ForecastStatus = "no_data"   // в закрытой части нет выручки
	ForecastFuture   ForecastStatus = "future"    // период целиком в будущем
)

// Forecast — результат run-rate прогноза выручки. Числа — источник истины; нарратив
// (A6) берёт их отсюда и обязан проговорить оговорку при LowConfidence/не-ok статусе.
type Forecast struct {
	Status        ForecastStatus
	Actual        float64 // выручка полных закрытых суток (Σ sum_all)
	DailyRate     float64 // Actual / CompleteDays — среднесуточный темп
	Projected     float64 // Actual + DailyRate*RemainingDays (== DailyRate*TotalDays)
	CompleteDays  int     // полных закрытых суток в основе ставки
	RemainingDays int     // сегодня (неполный) + будущие дни
	TotalDays     int     // всего дней в периоде
	LowConfidence bool    // <forecastConfidentDays полных суток → оговорка обязательна
}

// RunRateForecast строит прогноз выручки методом run-rate поверх дневного payment-ряда.
//
// rows — строки отчёта payment (ключи "date" YYYY-MM-DD и "sum_all"); fromISO/toISO —
// ПОЛНЫЙ целевой период (toISO — конец месяца, а не сегодня); asOf — «сегодня» в TZ
// тенанта (YYYY-MM-DD). Функция детерминированна: time.Now() внутри нет, asOf приходит
// снаружи — то же окно даёт тот же прогноз (тестируемость, зеркало прода).
//
// Ставка считается ТОЛЬКО по полным закрытым суткам (строго до asOf): сегодня — неполный
// день, его частичная выручка занизила бы темп. Знаменатель — по календарю, а не по числу
// строк ряда: payment выкидывает дни без движения, но закрытый день с нулём — реальные
// сутки (0 в числитель, 1 в знаменатель), иначе ставка завышается на «тихих» днях.
func RunRateForecast(rows []dooglys.Row, fromISO, toISO, asOf string) Forecast {
	from, errF := time.Parse(fcLayout, fromISO)
	to, errT := time.Parse(fcLayout, toISO)
	now, errN := time.Parse(fcLayout, asOf)
	if errF != nil || errT != nil || errN != nil || to.Before(from) {
		return Forecast{Status: ForecastNoData}
	}

	totalDays := daysInclusive(from, to)

	// Период целиком впереди: ни одни сутки ещё не наступили — прогнозировать нечем.
	if now.Before(from) {
		return Forecast{Status: ForecastFuture, TotalDays: totalDays, RemainingDays: totalDays}
	}

	// Период уже закрыт (сегодня позже конца): это факт за период, а не прогноз.
	if now.After(to) {
		actual := sumRevenueRange(rows, fromISO, toISO)
		return Forecast{
			Status:       ForecastFact,
			Actual:       round2(actual),
			Projected:    round2(actual),
			CompleteDays: totalDays,
			TotalDays:    totalDays,
		}
	}

	// Период идёт. Полные сутки — строго до сегодня (asOf−1 включительно).
	lastComplete := now.AddDate(0, 0, -1)
	completeDays := daysInclusive(from, lastComplete)
	if completeDays <= 0 {
		// asOf == from: только сегодняшний неполный день, ставку считать не из чего.
		return Forecast{Status: ForecastTooEarly, TotalDays: totalDays, RemainingDays: totalDays}
	}

	actual := sumRevenueRange(rows, fromISO, lastComplete.Format(fcLayout))
	remaining := totalDays - completeDays
	if actual == 0 {
		// Закрытая часть без выручки (пустая выборка / нули) — не выдаём «прогноз 0».
		return Forecast{
			Status:        ForecastNoData,
			CompleteDays:  completeDays,
			TotalDays:     totalDays,
			RemainingDays: remaining,
		}
	}

	rate := actual / float64(completeDays)
	return Forecast{
		Status:        ForecastOK,
		Actual:        round2(actual),
		DailyRate:     round2(rate),
		Projected:     round2(actual + rate*float64(remaining)),
		CompleteDays:  completeDays,
		RemainingDays: remaining,
		TotalDays:     totalDays,
		LowConfidence: completeDays < forecastConfidentDays,
	}
}

// daysInclusive — число календарных суток в [a, b] включительно. Даты разобраны как UTC
// (time.Parse без зоны) → разница кратна 24ч, без сюрпризов перехода на летнее время.
func daysInclusive(a, b time.Time) int {
	return int(b.Sub(a).Hours())/24 + 1
}

// ForecastEnvelope оборачивает Forecast в Envelope — единый формат для render/UI.
// Числа — источник истины из движка; нарратив строится здесь же (sandwich: LLM не нужна
// для детерминированного прогнозного числа). UI получает envelope.Type="forecast" и
// Summary с ключевыми метриками; Rows пустые — таблица не нужна, вся суть в нарративе.
// ForecastEnvelope оборачивает Forecast в Envelope — единый формат для render/UI.
// goal > 0 — явно названная пользователем цель (план); 0 — цель не названа, только проекция.
func ForecastEnvelope(f Forecast, fromISO, toISO, tenantID, currency, tz string, goal float64) envelope.Envelope {
	e := envelope.Envelope{
		Type:     "forecast",
		TenantID: tenantID,
		Period:   envelope.Period{From: fromISO, To: toISO, TZ: tz},
		Currency: currency,
		Meta:     map[string]any{"method": "forecast", "status": string(f.Status)},
	}
	switch f.Status {
	case ForecastFact:
		goalLine := goalVsFact(f.Actual, goal, currency)
		e.Narrative = fmt.Sprintf(
			"Период уже завершён — это факт, не прогноз.\nВыручка за период: %s.%s",
			render.Money(f.Actual, currency), goalLine)
		e.Summary = map[string]float64{"projected": f.Projected, "actual": f.Actual}
	case ForecastTooEarly:
		e.Narrative = "Период только начался — ещё нет ни одного полного дня, прогнозировать пока не из чего."
	case ForecastFuture:
		e.Narrative = "Период ещё не наступил — прогнозировать пока не из чего."
	case ForecastNoData:
		e.Narrative = "В закрытой части периода нет данных о выручке — прогноз невозможен."
	default: // ForecastOK
		disclaimer := ""
		if f.LowConfidence {
			disclaimer = fmt.Sprintf(
				" ⚠️ Оговорка: в основе прогноза всего %d %s — неполная неделя не покрывает цикл будни/выходные, погрешность высокая.",
				f.CompleteDays, dayWord(f.CompleteDays))
		}
		goalLine := goalVsProjected(f.Projected, goal, currency)
		e.Narrative = fmt.Sprintf(
			"Прогноз выручки к концу периода: %s (run-rate).\n"+
				"Факт за %d %s: %s → среднесуточный темп %s → ещё %d %s.%s%s",
			render.Money(f.Projected, currency),
			f.CompleteDays, dayWord(f.CompleteDays),
			render.Money(f.Actual, currency),
			render.Money(f.DailyRate, currency),
			f.RemainingDays, dayWord(f.RemainingDays),
			goalLine, disclaimer)
		e.Summary = map[string]float64{
			"projected":  f.Projected,
			"actual":     f.Actual,
			"daily_rate": f.DailyRate,
		}
	}
	return e
}

// goalVsProjected формирует строку сравнения прогноза с целью (для статуса ok).
func goalVsProjected(projected, goal float64, currency string) string {
	if goal <= 0 {
		return ""
	}
	gap := goal - projected
	if gap <= 0 {
		return fmt.Sprintf("\n✅ До плана %s дойдёшь — прогноз превысит на %s.",
			render.Money(goal, currency), render.Money(-gap, currency))
	}
	return fmt.Sprintf("\n❌ До плана %s не дойдёшь — разрыв %s.",
		render.Money(goal, currency), render.Money(gap, currency))
}

// goalVsFact формирует строку сравнения факта с целью (для статуса fact).
func goalVsFact(actual, goal float64, currency string) string {
	if goal <= 0 {
		return ""
	}
	gap := goal - actual
	if gap <= 0 {
		return fmt.Sprintf("\n✅ План %s выполнен — факт превысил на %s.",
			render.Money(goal, currency), render.Money(-gap, currency))
	}
	return fmt.Sprintf("\n❌ План %s не выполнен — недобор %s.",
		render.Money(goal, currency), render.Money(gap, currency))
}

// dayWord — склонение «день/дня/дней» для русского нарратива.
func dayWord(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 19:
		return "дней"
	case n%10 == 1:
		return "день"
	case n%10 >= 2 && n%10 <= 4:
		return "дня"
	default:
		return "дней"
	}
}

// sumRevenueRange суммирует sum_all по строкам payment с date в [loISO, hiISO]
// включительно. Сравнение строк ISO лексикографическое == хронологическое (YYYY-MM-DD).
func sumRevenueRange(rows []dooglys.Row, loISO, hiISO string) float64 {
	var s float64
	for _, r := range rows {
		d, _ := r["date"].(string)
		if d < loISO || d > hiISO {
			continue
		}
		v, _ := toFloat(r["sum_all"])
		s += v
	}
	return s
}
