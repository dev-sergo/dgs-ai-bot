package app

import (
	"regexp"
	"strconv"
	"strings"
)

// goalRe извлекает явно названную сумму-план из прогнозного запроса:
// «план 2 миллиона», «цель 500 тысяч», «выйти на 300к», «дойти до 1.5 млн».
// Группы: 1=число (пробелы внутри допустимы), 2=множитель (млн/тыс/к).
var goalRe = regexp.MustCompile(
	`(?:план[а-яё]*|цель|выйти\s+на|дойт[а-яё]*\s+до|дойд[а-яё]*\s+до)\s+(?:в\s+)?` +
		`(\d[\d\s]*(?:[.,]\d+)?)\s*` +
		`(млн|миллион[а-яё]*|тысяч[а-яё]*|тыс\.?|к)?`)

// extractGoal извлекает сумму-цель из текста запроса; 0 если цель не названа.
func extractGoal(query string) float64 {
	m := goalRe.FindStringSubmatch(strings.ToLower(query))
	if m == nil {
		return 0
	}
	numStr := strings.NewReplacer(" ", "", ",", ".").Replace(strings.TrimSpace(m[1]))
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil || val <= 0 {
		return 0
	}
	mult := strings.TrimSpace(m[2])
	switch {
	case strings.HasPrefix(mult, "млн"), strings.HasPrefix(mult, "миллион"):
		val *= 1_000_000
	case strings.HasPrefix(mult, "тыс"), mult == "к":
		val *= 1_000
	}
	return val
}
