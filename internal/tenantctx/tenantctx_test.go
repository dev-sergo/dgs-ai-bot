package tenantctx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLocationFromCachedLoc — Load кэширует *time.Location, Location() отдаёт его.
func TestLocationFromCachedLoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.json")
	doc := `{"tenants":[{"tenant_id":"t1","domain":"t1.example","timezone":"Europe/Moscow","currency":"RUB","currency_precision":2}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tn, ok := s.Lookup("t1")
	if !ok {
		t.Fatal("tenant t1 not found")
	}
	if got := tn.Location().String(); got != "Europe/Moscow" {
		t.Fatalf("cached loc = %q, want Europe/Moscow", got)
	}
}

// TestLocationLazyFromTimezone — тенант, собранный вручную (минуя Load, как дефолт
// «не найден» в app), всё равно должен резолвить зону из строки Timezone, а не UTC.
// Это и есть регресс-страж против тихого смещения периода на 3 часа.
func TestLocationLazyFromTimezone(t *testing.T) {
	tn := Tenant{Timezone: "Europe/Moscow", Currency: "RUB"}
	loc := tn.Location()
	if loc.String() != "Europe/Moscow" {
		t.Fatalf("lazy loc = %q, want Europe/Moscow", loc.String())
	}
	// Москва — UTC+3 без перехода на летнее время: фиксированное смещение.
	ref := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC).In(loc)
	_, offset := ref.Zone()
	if offset != 3*3600 {
		t.Fatalf("Moscow offset = %d s, want %d", offset, 3*3600)
	}
}

// TestLocationInvalidTimezone — мусорная/пустая зона безопасно падает на UTC.
func TestLocationInvalidTimezone(t *testing.T) {
	for _, tz := range []string{"", "Not/AZone"} {
		tn := Tenant{Timezone: tz}
		if got := tn.Location().String(); got != "UTC" {
			t.Fatalf("Location(%q) = %q, want UTC", tz, got)
		}
	}
}

// TestLoadInvalidTimezoneFallsBackToUTC — Load на невалидной зоне не падает,
// а кэширует UTC (графа в фикстуре с опечаткой не должна валить старт).
func TestLoadInvalidTimezoneFallsBackToUTC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.json")
	doc := `{"tenants":[{"tenant_id":"bad","domain":"bad.example","timezone":"Mars/Phobos"}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tn, _ := s.Lookup("bad")
	if got := tn.Location().String(); got != "UTC" {
		t.Fatalf("invalid tz loc = %q, want UTC", got)
	}
}

// TestLookupByIDAndDomain — поиск идёт и по tenant_id, и по domain.
func TestLookupByIDAndDomain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.json")
	doc := `{"tenants":[{"tenant_id":"id1","domain":"shop.dooglys.com","timezone":"UTC"}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := s.Lookup("id1"); !ok {
		t.Error("lookup by tenant_id failed")
	}
	if _, ok := s.Lookup("shop.dooglys.com"); !ok {
		t.Error("lookup by domain failed")
	}
	if _, ok := s.Lookup("nope"); ok {
		t.Error("lookup of unknown id should be false")
	}
}

// TestLoadMissingFile — отсутствующий файл даёт ошибку, а не панику.
func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
