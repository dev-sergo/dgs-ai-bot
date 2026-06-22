// Package dooglys — HTTP-клиент Dooglys на основе cookie-сессии.
// Реализует Client через SSR-HTML GridView, потому что JSON-API у Dooglys
// отдаёт только агрегат-сводки (payment-total), а не строчные данные.
package dooglys

import (
	"compress/gzip"
	"context"
	"fmt"
	htmlpkg "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// scalarParams — фильтры, которые Dooglys принимает как одиночное значение
// (без [] в имени параметра). Все остальные — массивы.
var scalarParams = map[string]bool{
	"order_number":        true,
	"cost_from":           true,
	"cost_to":             true,
	"include_zero_price":  true,
	"phone_number":        true,
}

// fieldOverrides задаёт machine-ключи для колонок, у которых нет атрибута data-sort
// в HTML (или data-sort не соответствует нужному ключу).
// Пустая строка "" — пропустить колонку (напр. служебные "Детали").
var fieldOverrides = map[string]map[int]string{
	"payment": {
		1:  "kol_vo_chekov", // Кол-во чеков
		5:  "sum_cash",      // Наличные
		6:  "onlayn",        // Онлайн
		7:  "sbp",           // СБП
		9:  "sredniy_chek",  // Средний чек
		10: "",              // "Детали" — служебная
	},
	"products": {
		1: "artikul", // Артикул
	},
	"paycheck": {
		1:  "order_number", // №Заказа
		5:  "check_type",   // Тип чека
		7:  "tip_oplaty",   // Тип оплаты
		11: "",             // "ПодробнееДетали" — служебная
	},
	"orders": {
		1:  "istochnik",        // Источник
		2:  "kassir",           // Кассир (PII)
		3:  "pokupatel",        // Покупатель (PII)
		4:  "torgovaya_tochka", // Торговая точка
		6:  "delivered_at",     // Доставлен
		7:  "completed_at",     // Завершен
		10: "status",           // Статус
		11: "order_type",       // Тип заказа
		12: "tip_oplaty",       // Тип оплаты
		13: "delivery_time",    // Доставка ко времени
		14: "return_reason",    // Причина возврата
		15: "fio",              // ФИО (PII)
		16: "address",          // Адрес
		17: "coordinates",      // Координаты
		18: "",                 // "ПодробнееДетали" — служебная
	},
}

// Скомпилированные регулярные выражения для парсинга GridView-таблицы.
var (
	reGridChunk = regexp.MustCompile(
		`(?is)<div[^>]*class="[^"]*(?:grid-view|table__wrapper|report-content)[^"]*"[^>]*>.*?</table>|` +
			`<div[^>]*id="reports"[^>]*>.*?</table>`)
	reTHead    = regexp.MustCompile(`(?is)<thead[^>]*>(.*?)</thead>`)
	reTBody    = regexp.MustCompile(`(?is)<tbody[^>]*>(.*?)</tbody>`)
	reTFoot    = regexp.MustCompile(`(?is)<tfoot[^>]*>(.*?)</tfoot>`)
	reTR       = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	reTH       = regexp.MustCompile(`(?is)<th[^>]*>(.*?)</th>`)
	reTD       = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	reDataSort = regexp.MustCompile(`(?i)data-sort="([^"]*)"`)
	reTagStrip = regexp.MustCompile(`<[^>]+>`)
	reSpace    = regexp.MustCompile(`\s+`)
	reDateDMY  = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{4})$`)
	// Парсинг <select>-фильтров формы отчёта: имя параметра BaseReportForm[<param>][]
	// и его <option value="uuid">Имя</option>. Источник живых uuid справочников.
	reSelectBlock = regexp.MustCompile(`(?is)<select[^>]*\bname="BaseReportForm\[([a-z_]+)\][^"]*"[^>]*>(.*?)</select>`)
	reOptionVal   = regexp.MustCompile(`(?is)<option[^>]*\bvalue="([^"]*)"[^>]*>(.*?)</option>`)
	reNumeric  = regexp.MustCompile(`^-?[\d  ]+(?:[.,]\d+)?\s*[₽%]?\s*$`)
)

// HTMLClient реализует Client через HTTP + Cookie-сессию.
// cookie — полное значение заголовка Cookie (advanced-backend=...; _csrf-backend=...; _identity-backend=...).
type HTMLClient struct {
	base   string
	cookie string
	http   *http.Client
}

// NewHTMLClient создаёт клиент. base — корень Dooglys (https://google.dooglys.com).
func NewHTMLClient(base, cookie string) *HTMLClient {
	return &HTMLClient{
		base:   strings.TrimRight(base, "/"),
		cookie: cookie,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch реализует Client: запрашивает SSR-страницу отчёта и возвращает нормализованные строки.
func (c *HTMLClient) Fetch(ctx context.Context, q Query) (Result, error) {
	rawURL := c.buildURL(q)
	html, err := c.fetchHTML(ctx, rawURL)
	if err != nil {
		return Result{}, fmt.Errorf("dooglys htmlclient: %w", err)
	}
	if strings.Contains(html, "site/login") || strings.Contains(html, "LoginForm") {
		return Result{}, fmt.Errorf("dooglys htmlclient: session expired, re-login required")
	}
	if !strings.Contains(html, "grid-view") && !strings.Contains(html, "table-wrap") &&
		!strings.Contains(html, "table__wrapper") {
		return Result{}, fmt.Errorf("dooglys htmlclient: report %q — no grid found (empty or unsupported)", q.Report)
	}

	rows, filtersApplied, filtersSkipped := parseGrid(html, q)
	return Result{
		Report:         q.Report,
		Rows:           rows,
		FiltersApplied: filtersApplied,
		FiltersSkipped: filtersSkipped,
	}, nil
}

// FetchSelects возвращает живые справочники из <select>-фильтров HTML-формы отчёта.
// Ключ карты — имя параметра (locality_id, sale_point_id, ...), значение — список
// {UUID, Name} из <option>. Используется resolver.NewLiveStore вместо офлайн grid-снимков.
func (c *HTMLClient) FetchSelects(ctx context.Context, report string) (map[string][]SelectOption, error) {
	html, err := c.fetchHTML(ctx, c.base+"/report/"+report)
	if err != nil {
		return nil, fmt.Errorf("dooglys htmlclient selects: %w", err)
	}
	if strings.Contains(html, "site/login") || strings.Contains(html, "LoginForm") {
		return nil, fmt.Errorf("dooglys htmlclient selects: session expired, re-login required")
	}
	sel := parseSelects(html)
	if len(sel) == 0 {
		return nil, fmt.Errorf("dooglys htmlclient selects: no filter <select> found in report %q", report)
	}
	return sel, nil
}

// parseSelects извлекает справочники из <select name="BaseReportForm[<param>][]">-блоков.
// Пустые option-значения (плейсхолдеры «Все»/«—») пропускаются.
func parseSelects(html string) map[string][]SelectOption {
	out := map[string][]SelectOption{}
	for _, sm := range reSelectBlock.FindAllStringSubmatch(html, -1) {
		param, body := sm[1], sm[2]
		var opts []SelectOption
		seen := map[string]bool{}
		for _, om := range reOptionVal.FindAllStringSubmatch(body, -1) {
			uuid := strings.TrimSpace(om[1])
			name := cellText(om[2])
			if uuid == "" || name == "" || seen[uuid] {
				continue
			}
			seen[uuid] = true
			opts = append(opts, SelectOption{UUID: uuid, Name: name})
		}
		if len(opts) > 0 {
			out[param] = opts
		}
	}
	return out
}

// buildURL строит URL отчёта с параметрами периода и фильтров.
func (c *HTMLClient) buildURL(q Query) string {
	params := url.Values{}
	params.Set("BaseReportForm[period]", q.From+"-"+q.To)

	for _, f := range q.Filters {
		if scalarParams[f.Param] {
			// Скалярный параметр: одно значение без []
			if len(f.Names) > 0 {
				params.Set("BaseReportForm["+f.Param+"]", f.Names[0])
			}
			continue
		}
		// Массивный параметр: BaseReportForm[param][]
		key := "BaseReportForm[" + f.Param + "][]"
		if len(f.UUIDs) > 0 {
			for _, uuid := range f.UUIDs {
				params.Add(key, uuid)
			}
		} else {
			for _, name := range f.Names {
				params.Add(key, name)
			}
		}
	}

	return c.base + "/report/" + q.Report + "?" + params.Encode()
}

// fetchHTML выполняет HTTP GET с Cookie и возвращает декодированный HTML.
func (c *HTMLClient) fetchHTML(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:131.0) Gecko/20100101 Firefox/131.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Cookie", c.cookie)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("gzip: %w", err)
		}
		defer gr.Close()
		reader = gr
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// parseGrid извлекает строки из GridView-таблицы HTML-страницы Dooglys.
// Возвращает нормализованные строки + списки применённых и пропущенных фильтров.
func parseGrid(html string, q Query) ([]Row, []string, []string) {
	// Вырезаем блок с таблицей, чтобы не цепляться за layout-таблицы страницы.
	chunk := html
	if m := reGridChunk.FindString(html); m != "" {
		chunk = m
	}

	// Определяем ключи колонок по <thead>.
	cols := extractColumns(chunk, q.Report)

	// Строки из <tbody>.
	var rows []Row
	if bm := reTBody.FindStringSubmatch(chunk); len(bm) > 1 {
		for _, trm := range reTR.FindAllStringSubmatch(bm[1], -1) {
			cells := extractCells(trm[1])
			row := zipRow(cols, cells)
			if len(row) > 0 {
				rows = append(rows, row)
			}
		}
	}

	// Определяем, какие фильтры были применены (есть в запросе и нашлись строки).
	var applied, skipped []string
	for _, f := range q.Filters {
		if len(rows) > 0 {
			applied = append(applied, f.Field)
		} else {
			skipped = append(skipped, f.Field)
		}
	}

	return rows, applied, skipped
}

// extractColumns вытаскивает упорядоченный список machine-ключей колонок из <thead>.
func extractColumns(chunk, report string) []string {
	overrides := fieldOverrides[report]

	hm := reTHead.FindStringSubmatch(chunk)
	if len(hm) < 2 {
		// Нет thead — возвращаем ключи по позиционной карте если есть
		if overrides != nil {
			max := 0
			for k := range overrides { if k > max { max = k } }
			cols := make([]string, max+1)
			for k, v := range overrides { cols[k] = v }
			return cols
		}
		return nil
	}

	// Извлекаем <th> из первого <tr> thead
	trm := reTR.FindStringSubmatch(hm[1])
	if len(trm) < 2 {
		return nil
	}
	ths := reTH.FindAllStringSubmatch(trm[1], -1)

	cols := make([]string, len(ths))
	for i, thm := range ths {
		thHTML := thm[1]

		// Ищем data-sort на <th> или внутри <a>
		field := ""
		if sm := reDataSort.FindStringSubmatch(thm[0]); len(sm) > 1 {
			field = sm[1]
		} else if sm := reDataSort.FindStringSubmatch(thHTML); len(sm) > 1 {
			field = sm[1]
		}

		// Убираем минус-префикс (индикатор направления сортировки)
		field = strings.TrimPrefix(field, "-")

		// Позиционный override имеет приоритет над data-sort когда явно задан
		if overrides != nil {
			if ov, ok := overrides[i]; ok {
				cols[i] = ov
				continue
			}
		}

		cols[i] = field
	}
	return cols
}

// extractCells вытаскивает текстовое содержимое <td>-ячеек из HTML строки <tr>.
func extractCells(trHTML string) []string {
	tds := reTD.FindAllStringSubmatch(trHTML, -1)
	cells := make([]string, len(tds))
	for i, tdm := range tds {
		cells[i] = cellText(tdm[1])
	}
	return cells
}

// cellText чистит HTML из содержимого ячейки: убирает теги, декодирует сущности.
func cellText(raw string) string {
	s := reTagStrip.ReplaceAllString(raw, " ")
	s = htmlpkg.UnescapeString(s) // &quot; → ", &amp; → &, &nbsp; → пробел и т.д.
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// zipRow объединяет заголовки колонок со значениями ячеек в нормализованный Row.
// Колонки с пустым ключом (служебные) пропускаются.
func zipRow(cols []string, cells []string) Row {
	row := make(Row, len(cols))
	for i, key := range cols {
		if key == "" {
			continue // служебная колонка (Детали, ПодробнееДетали и т.п.)
		}
		if i >= len(cells) {
			break
		}
		row[key] = normalizeCell(cells[i])
	}
	return row
}

// normalizeCell нормализует текстовое значение ячейки:
// — DD.MM.YYYY → ISO YYYY-MM-DD (чтобы совпадать с форматом фикстур)
// — числа (₽/пробелы/запятая) → float64
// — пустые/"—" → nil
// — остальное → string
func normalizeCell(s string) any {
	s = strings.ReplaceAll(s, " ", " ")
	s = strings.TrimSpace(s)

	switch s {
	case "", "—", "Ничего не найдено.":
		return nil
	}

	// Дата DD.MM.YYYY → YYYY-MM-DD
	if m := reDateDMY.FindStringSubmatch(s); m != nil {
		return m[3] + "-" + m[2] + "-" + m[1]
	}

	// Число/процент: убираем ₽, %, пробелы-разделители, меняем запятую на точку
	if reNumeric.MatchString(s) {
		v := strings.TrimRight(s, "₽% ")
		v = strings.ReplaceAll(v, " ", "")
		v = strings.ReplaceAll(v, " ", "")
		v = strings.ReplaceAll(v, ",", ".")
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}

	return s
}
