package dooglys

import "testing"

const selectHTML = `
<form>
<select id="basereportform-locality_id" class="form-control" name="BaseReportForm[locality_id][]" multiple>
  <option value="">Все</option>
  <option value="0f585eb1-6413-4f42-bf8a-257142acf537">Выкса осн</option>
  <option value="6d095a20-c4b7-42d0-a999-ea7fb8fcf74d">Казань</option>
</select>
<select id="basereportform-sale_point_id" class="form-control" name="BaseReportForm[sale_point_id][]" multiple>
  <option value="4b837e5a-4367-488c-91be-3b349157d965">Выкса</option>
  <option value="06b8a877-7637-4bbc-b5e6-02fcdc6fd77a">ВЫКСА</option>
</select>
</form>`

func TestParseSelects(t *testing.T) {
	got := parseSelects(selectHTML)

	loc := got["locality_id"]
	if len(loc) != 2 { // пустой плейсхолдер «Все» отброшен
		t.Fatalf("locality_id: %d опций, want 2: %+v", len(loc), loc)
	}
	if loc[0].UUID != "0f585eb1-6413-4f42-bf8a-257142acf537" || loc[0].Name != "Выкса осн" {
		t.Errorf("первая опция locality неверна: %+v", loc[0])
	}

	if sp := got["sale_point_id"]; len(sp) != 2 {
		t.Errorf("sale_point_id: %d опций, want 2: %+v", len(sp), sp)
	}
}

// На реальном снимке payment.html (если есть) форма содержит locality_id и sale_point_id.
func TestParseSelects_CapturedPayment(t *testing.T) {
	html := capturedHTML(t, "payment.html") // t.Skip, если файла нет
	got := parseSelects(html)
	for _, param := range []string{"locality_id", "sale_point_id"} {
		if len(got[param]) == 0 {
			t.Errorf("в payment.html не найдено опций для %s", param)
		}
	}
}
