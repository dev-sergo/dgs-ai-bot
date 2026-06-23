// Package pipeval — full-pipeline бенчмарк: прогон запросов через app.Ask и
// сверка ИТОГОВОГО ОТВЕТА (числа, текст, нарратив, отсутствие утечек), а не плана.
//
// В отличие от пакета eval (он проверяет план планировщика), pipeval проверяет то,
// что реально получает пользователь: правильность чисел из движка, непустоту,
// формат, наличие нарратива для аналитики, корректный отказ для off_topic.
// Данные берутся из детерминированных фикстур (dooglys.FixtureClient), поэтому
// для кейса с известными данными можно сверять ТОЧНОЕ значение.
package pipeval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"dgsbot/internal/app"
)

// LoadCases читает набор кейсов из jsonl-файла (по одному JSON-объекту на строку).
func LoadCases(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []Case
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c Case
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("строка %q: %w", string(line), err)
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

// Expect — ожидаемые свойства ОТВЕТА (проверяются только заданные поля).
type Expect struct {
	Intent       string             `json:"intent,omitempty"`         // report|help|smalltalk|off_topic
	Clarify      *bool              `json:"clarify,omitempty"`        // ожидается переспрос (NeedClarify)
	Envelope     *bool              `json:"envelope,omitempty"`       // должен ли быть построен envelope
	NonEmptyText bool               `json:"non_empty_text,omitempty"` // текст ответа не пустой
	Contains     []string           `json:"contains,omitempty"`       // подстроки, которые ДОЛЖНЫ быть в тексте (все)
	ContainsAny  []string           `json:"contains_any,omitempty"`   // хотя бы одна из подстрок (must-mention для совета)
	MentionsNum  *bool              `json:"mentions_number,omitempty"` // в тексте есть цифра (совет подкреплён числом)
	NotContains  []string           `json:"not_contains,omitempty"`   // подстроки, которых быть НЕ должно (утечки)
	Summary      map[string]float64 `json:"summary,omitempty"`        // точные значения envelope.Summary
	Narrative    *bool              `json:"narrative,omitempty"`      // наличие нарратива (class B)
	Rows         *int               `json:"rows,omitempty"`           // ожидаемое число строк (напр. top_n=1)
}

// Case — один кейс набора. History — предшествующие реплики пользователя в той же
// сессии (для проверки многоходовых сценариев, в т.ч. инъекций из истории диалога).
type Case struct {
	History []string `json:"history,omitempty"`
	Query   string   `json:"query"`
	Expect  Expect   `json:"expect"`
}

// Result — итог по кейсу.
type Result struct {
	Query     string
	Answer    app.Answer
	Mismatch  []string
	LatencyMS int64
	Err       error
}

// Pass — кейс прошёл (нет ошибки и расхождений).
func (r Result) Pass() bool { return r.Err == nil && len(r.Mismatch) == 0 }

// piiPatterns — автоскан утечек PII в тексте ответа (defense-in-depth).
// Только e-mail: в легальном отчёте символа @ не бывает, ложных срабатываний нет.
// Телефоны/ФИО проверяются точечно через not_contains в PII-кейсах (сырые суммы
// выручки могут давать длинные цепочки цифр — авто-regex по цифрам ненадёжен).
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`), // email
}

// digitRe — детект «совет подкреплён числом»: любая цифра в тексте ответа.
var digitRe = regexp.MustCompile(`[0-9]`)

// Run прогоняет кейсы через готовый App (сессия на каждый кейс — без утечки истории).
func Run(ctx context.Context, a *app.App, tenantID string, cases []Case) []Result {
	out := make([]Result, 0, len(cases))
	for i, c := range cases {
		start := time.Now()
		sess := fmt.Sprintf("pipeval-%d", i)
		// Проигрываем предысторию в той же сессии — она ляжет в историю диалога.
		var setupErr error
		for _, h := range c.History {
			if _, err := a.Ask(ctx, tenantID, sess, h); err != nil {
				setupErr = err
				break
			}
		}
		r := Result{Query: c.Query, LatencyMS: time.Since(start).Milliseconds()}
		if setupErr != nil {
			r.Err = setupErr
			out = append(out, r)
			continue
		}
		ans, err := a.Ask(ctx, tenantID, sess, c.Query)
		r.Answer, r.Err = ans, err
		r.LatencyMS = time.Since(start).Milliseconds()
		if err == nil {
			r.Mismatch = Check(ans, c.Expect)
		}
		out = append(out, r)
	}
	return out
}

// Check сверяет ответ с ожиданиями; возвращает список расхождений.
func Check(ans app.Answer, e Expect) []string {
	var m []string
	add := func(f string, args ...any) { m = append(m, fmt.Sprintf(f, args...)) }

	if e.Intent != "" {
		got := ans.Plan.EffectiveIntent()
		if got != e.Intent {
			add("intent=%s ожидался %s", got, e.Intent)
		}
	}
	if e.Clarify != nil && ans.Validation.NeedClarify != *e.Clarify {
		add("clarify=%v ожидался %v", ans.Validation.NeedClarify, *e.Clarify)
	}
	if e.Envelope != nil {
		has := ans.Envelope != nil
		if has != *e.Envelope {
			add("envelope=%v ожидался %v", has, *e.Envelope)
		}
	}
	if e.NonEmptyText && strings.TrimSpace(ans.Text) == "" {
		add("текст ответа пустой")
	}
	for _, sub := range e.Contains {
		if !strings.Contains(ans.Text, sub) {
			add("в тексте нет %q", sub)
		}
	}
	if len(e.ContainsAny) > 0 {
		hit := false
		for _, sub := range e.ContainsAny {
			if strings.Contains(ans.Text, sub) {
				hit = true
				break
			}
		}
		if !hit {
			add("в тексте нет ни одной из %v", e.ContainsAny)
		}
	}
	if e.MentionsNum != nil {
		has := digitRe.MatchString(ans.Text)
		if has != *e.MentionsNum {
			add("mentions_number=%v ожидался %v", has, *e.MentionsNum)
		}
	}
	for _, sub := range e.NotContains {
		if strings.Contains(ans.Text, sub) {
			add("в тексте найдено запрещённое %q", sub)
		}
	}
	// Автоскан PII-утечек — всегда, независимо от ожиданий.
	for _, re := range piiPatterns {
		if hit := re.FindString(ans.Text); hit != "" {
			add("возможная утечка PII в тексте: %q", hit)
		}
	}
	for key, want := range e.Summary {
		if ans.Envelope == nil {
			add("нет summary[%s]: envelope не построен", key)
			continue
		}
		got, ok := ans.Envelope.Summary[key]
		if !ok {
			add("нет summary[%s] в envelope", key)
			continue
		}
		if got != want {
			add("summary[%s]=%v ожидалось %v", key, got, want)
		}
	}
	if e.Narrative != nil {
		has := ans.Envelope != nil && strings.TrimSpace(ans.Envelope.Narrative) != ""
		if has != *e.Narrative {
			add("narrative=%v ожидался %v", has, *e.Narrative)
		}
	}
	if e.Rows != nil {
		got := 0
		if ans.Envelope != nil {
			got = len(ans.Envelope.Rows)
		}
		if got != *e.Rows {
			add("rows=%d ожидалось %d", got, *e.Rows)
		}
	}
	return m
}

// Stats — сводка по прогону.
type Stats struct {
	Total, Passed, Errors  int
	LatP50, LatP95, LatMax int64
}

// Summarize считает агрегаты.
func Summarize(rs []Result) Stats {
	s := Stats{Total: len(rs)}
	lats := make([]int64, 0, len(rs))
	for _, r := range rs {
		if r.Err != nil {
			s.Errors++
		}
		if r.Pass() {
			s.Passed++
		}
		lats = append(lats, r.LatencyMS)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	s.LatP50 = percentile(lats, 50)
	s.LatP95 = percentile(lats, 95)
	if len(lats) > 0 {
		s.LatMax = lats[len(lats)-1]
	}
	return s
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
