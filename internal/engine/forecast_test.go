package engine

import (
	"testing"

	"dgsbot/internal/dooglys"
)

// dayRows строит payment-ряд: по строке на каждый день из days с выручкой sum_all.
func dayRows(days map[string]float64) []dooglys.Row {
	rows := make([]dooglys.Row, 0, len(days))
	for d, v := range days {
		rows = append(rows, dooglys.Row{"date": d, "sum_all": v})
	}
	return rows
}

func TestRunRateForecast_MidPeriod(t *testing.T) {
	// Месяц 01–10 июня, сегодня 06-е. Полные сутки 01–05 по 1000 → база 5000/5 дней.
	// Сегодняшние частичные 300 в базу НЕ входят (date 06 > последнего полного 05).
	rows := dayRows(map[string]float64{
		"2026-06-01": 1000, "2026-06-02": 1000, "2026-06-03": 1000,
		"2026-06-04": 1000, "2026-06-05": 1000, "2026-06-06": 300,
	})
	f := RunRateForecast(rows, "2026-06-01", "2026-06-10", "2026-06-06")

	if f.Status != ForecastOK {
		t.Fatalf("Status = %q, want ok", f.Status)
	}
	if f.CompleteDays != 5 || f.TotalDays != 10 || f.RemainingDays != 5 {
		t.Errorf("days = %d/%d/%d, want complete=5 total=10 remaining=5",
			f.CompleteDays, f.TotalDays, f.RemainingDays)
	}
	if f.Actual != 5000 {
		t.Errorf("Actual = %v, want 5000 (сегодняшние 300 исключены)", f.Actual)
	}
	if f.DailyRate != 1000 {
		t.Errorf("DailyRate = %v, want 1000", f.DailyRate)
	}
	if f.Projected != 10000 { // 5000 + 1000*5
		t.Errorf("Projected = %v, want 10000", f.Projected)
	}
	if !f.LowConfidence { // 5 полных суток < недели
		t.Error("LowConfidence = false, want true (меньше недели данных)")
	}
}

func TestRunRateForecast_ConfidentWeek(t *testing.T) {
	// 8 полных суток по 1000 → дисклеймер мягкий (>= недели).
	days := map[string]float64{}
	for _, d := range []string{"01", "02", "03", "04", "05", "06", "07", "08"} {
		days["2026-06-"+d] = 1000
	}
	f := RunRateForecast(dayRows(days), "2026-06-01", "2026-06-30", "2026-06-09")

	if f.Status != ForecastOK || f.CompleteDays != 8 {
		t.Fatalf("Status=%q CompleteDays=%d, want ok/8", f.Status, f.CompleteDays)
	}
	if f.LowConfidence {
		t.Error("LowConfidence = true, want false (8 суток >= недели)")
	}
	if f.Projected != 30000 { // 8000 + 1000*22
		t.Errorf("Projected = %v, want 30000", f.Projected)
	}
}

func TestRunRateForecast_SingleDay(t *testing.T) {
	rows := dayRows(map[string]float64{"2026-06-01": 1500})
	f := RunRateForecast(rows, "2026-06-01", "2026-06-30", "2026-06-02")

	if f.Status != ForecastOK || f.CompleteDays != 1 {
		t.Fatalf("Status=%q CompleteDays=%d, want ok/1", f.Status, f.CompleteDays)
	}
	if !f.LowConfidence {
		t.Error("LowConfidence = false, want true (экстраполяция от одного дня)")
	}
	if f.Projected != 45000 { // 1500 * 30
		t.Errorf("Projected = %v, want 45000", f.Projected)
	}
}

func TestRunRateForecast_TooEarly(t *testing.T) {
	// Сегодня == начало периода: ни одних полных суток.
	f := RunRateForecast(nil, "2026-06-01", "2026-06-30", "2026-06-01")
	if f.Status != ForecastTooEarly {
		t.Errorf("Status = %q, want too_early", f.Status)
	}
	if f.CompleteDays != 0 {
		t.Errorf("CompleteDays = %d, want 0", f.CompleteDays)
	}
}

func TestRunRateForecast_ClosedIsFact(t *testing.T) {
	// Период целиком в прошлом → факт, не прогноз: Projected == Actual.
	rows := dayRows(map[string]float64{
		"2026-05-10": 2000, "2026-05-11": 3000,
	})
	f := RunRateForecast(rows, "2026-05-01", "2026-05-31", "2026-06-25")
	if f.Status != ForecastFact {
		t.Fatalf("Status = %q, want fact", f.Status)
	}
	if f.Actual != 5000 || f.Projected != 5000 {
		t.Errorf("Actual/Projected = %v/%v, want 5000/5000", f.Actual, f.Projected)
	}
	if f.RemainingDays != 0 {
		t.Errorf("RemainingDays = %d, want 0", f.RemainingDays)
	}
}

func TestRunRateForecast_Future(t *testing.T) {
	f := RunRateForecast(nil, "2026-07-01", "2026-07-31", "2026-06-25")
	if f.Status != ForecastFuture {
		t.Errorf("Status = %q, want future", f.Status)
	}
}

func TestRunRateForecast_NoData(t *testing.T) {
	// Период идёт, полные сутки есть, но выручки в закрытой части нет.
	f := RunRateForecast(nil, "2026-06-01", "2026-06-30", "2026-06-10")
	if f.Status != ForecastNoData {
		t.Fatalf("Status = %q, want no_data", f.Status)
	}
	if f.CompleteDays != 9 {
		t.Errorf("CompleteDays = %d, want 9", f.CompleteDays)
	}
}

func TestRunRateForecast_ExcludesTodayPartial(t *testing.T) {
	// Контроль изоляции «исключаем сегодня»: без сегодняшней строки и с ней
	// прогноз одинаков — частичная выручка дня asOf в базу не попадает.
	base := map[string]float64{"2026-06-01": 1000, "2026-06-02": 1000}
	without := RunRateForecast(dayRows(base), "2026-06-01", "2026-06-30", "2026-06-03")

	withToday := map[string]float64{"2026-06-01": 1000, "2026-06-02": 1000, "2026-06-03": 999}
	with := RunRateForecast(dayRows(withToday), "2026-06-01", "2026-06-30", "2026-06-03")

	if without.Projected != with.Projected || without.Actual != with.Actual {
		t.Errorf("сегодняшняя частичная выручка повлияла на прогноз: %+v vs %+v", without, with)
	}
}
