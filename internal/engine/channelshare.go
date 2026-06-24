package engine

import (
	"fmt"
	"sort"
	"strings"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

// cashlessKeys — безналичные каналы (всё, кроме наличных). «Доля безналичных» = их сумма.
var cashlessKeys = []string{"sum_card", "onlayn", "sbp"}

// ChannelShare — детерминированная доля каналов оплаты ЗА ОДИН период (не сравнение).
// Отвечает на «доля безналичных / онлайн / по карте»: считает процент каждого канала от
// суммы положительных каналов и выделяет запрошенный (focusKeys). Доли в [0,100], сумма=100%
// (нормировка по положительной базе — см. channelMix). Нарратив строится здесь же, без LLM:
// это факт из чисел, а не объяснение. focusKeys — подмножество каналов, по которым пользователь
// спросил долю (один канал, либо безналичная группа); пусто → показываем общую структуру.
func ChannelShare(res dooglys.Result, focusKeys []string,
	tenantID, currency string, period envelope.Period) envelope.Envelope {

	type chRow struct {
		key, label string
		now        float64
	}
	var pos []chRow
	var posSum float64
	for _, ch := range channelDefs {
		now := sumField(res.Rows, ch.key)
		if now <= 0 {
			continue
		}
		pos = append(pos, chRow{ch.key, ch.label, now})
		posSum += now
	}

	focus := map[string]bool{}
	for _, k := range focusKeys {
		focus[k] = true
	}

	rows := make([]map[string]any, 0, len(pos))
	var focusShare float64
	for _, ch := range pos {
		share := shareOf(ch.now, posSum)
		rows = append(rows, map[string]any{
			"channel": ch.label, "amount": round2(ch.now), "share": share,
		})
		if focus[ch.key] {
			focusShare += share
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, _ := toFloat(rows[i]["amount"])
		b, _ := toFloat(rows[j]["amount"])
		return a > b
	})

	env := envelope.Envelope{
		Type:     "payment_channel_share",
		TenantID: tenantID,
		Period:   period,
		Currency: currency,
		Columns: []envelope.Column{
			{Key: "channel", Label: "Канал", Unit: ""},
			{Key: "amount", Label: "Сумма", Unit: "RUB"},
			{Key: "share", Label: "Доля", Unit: "percent"},
		},
		Rows: rows,
		Meta: map[string]any{"method": "channel_share"},
	}
	if len(rows) > 0 {
		env.Narrative = channelShareNarrative(focusKeys, round2(focusShare), rows)
	}
	return env
}

// channelShareNarrative — детерминированная подводка: выделенная доля + полная структура.
func channelShareNarrative(focusKeys []string, focusShare float64, rows []map[string]any) string {
	breakdown := make([]string, 0, len(rows))
	for _, r := range rows {
		label, _ := r["channel"].(string)
		share, _ := toFloat(r["share"])
		breakdown = append(breakdown, fmt.Sprintf("%s %s", strings.ToLower(label), pct1(share)))
	}
	all := strings.Join(breakdown, ", ")
	if len(focusKeys) == 0 {
		return "Структура оплат за период: " + all + "."
	}
	return fmt.Sprintf("%s за период — %s выручки (%s).",
		channelFocusLabel(focusKeys), pct1(focusShare), all)
}

// channelFocusLabel — название выделенного среза: один канал → его имя, безналичная
// группа → «Безналичные», иначе перечисление.
func channelFocusLabel(keys []string) string {
	if len(keys) == 1 {
		return channelLabel(keys[0])
	}
	if sameKeySet(keys, cashlessKeys) {
		return "Безналичные"
	}
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = channelLabel(k)
	}
	return strings.Join(labels, "+")
}

func channelLabel(key string) string {
	for _, ch := range channelDefs {
		if ch.key == key {
			return ch.label
		}
	}
	return key
}

func sameKeySet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, k := range a {
		set[k] = true
	}
	for _, k := range b {
		if !set[k] {
			return false
		}
	}
	return true
}

// pct1 — процент в RU-стиле с одним знаком после запятой (нарратив; в таблице — render).
func pct1(v float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1) + "%"
}
