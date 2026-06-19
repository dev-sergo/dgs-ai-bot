package resolver

import "testing"

const fixturesDir = "../../docs/contracts/fixtures"

func store(t *testing.T) *Store {
	t.Helper()
	return Load(fixturesDir)
}

func TestResolveSalePointUnique(t *testing.T) {
	m, err := store(t).Resolve("sale_point", "Казанский вокзал")
	if err != nil {
		t.Fatalf("не резолвится уникальная точка: %v", err)
	}
	if m.UUID == "" {
		t.Error("ожидался непустой uuid")
	}
}

func TestResolveLocalityByShortName(t *testing.T) {
	// У региона есть краткое имя «НН» (вторая колонка).
	m, err := store(t).Resolve("locality", "НН")
	if err != nil {
		t.Fatalf("не резолвится регион по краткому имени: %v", err)
	}
	if m.UUID == "" {
		t.Error("ожидался непустой uuid региона")
	}
}

func TestResolveNotFound(t *testing.T) {
	_, err := store(t).Resolve("sale_point", "Точки-такой-нет-12345")
	re, ok := err.(*ResolveError)
	if !ok || re.Ambiguous {
		t.Fatalf("ожидалась ошибка not-found, получено: %v", err)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	// «Авто» встречается в нескольких точках (АвтоPizza, Автосуши №1).
	_, err := store(t).Resolve("sale_point", "Авто")
	re, ok := err.(*ResolveError)
	if !ok || !re.Ambiguous {
		t.Fatalf("ожидалась неоднозначность, получено: %v", err)
	}
	if len(re.Candidates) < 2 {
		t.Errorf("ожидалось ≥2 кандидата, получено: %v", re.Candidates)
	}
}
