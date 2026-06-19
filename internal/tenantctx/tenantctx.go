// Package tenantctx — справочник тенантов (таймзона, валюта, точки).
//
// В MVP грузится из docs/contracts/fixtures/tenants.example.json. В проде заменится
// на реальный источник, отдающий контекст тенанта по идентификатору из пре-слоя авторизации.
package tenantctx

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// SalePoint — торговая точка тенанта.
type SalePoint struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Locality string `json:"locality"`
}

// Tenant — контекст тенанта.
type Tenant struct {
	TenantID          string      `json:"tenant_id"`
	Domain            string      `json:"domain"`
	Timezone          string      `json:"timezone"`
	Currency          string      `json:"currency"`
	CurrencyPrecision int         `json:"currency_precision"`
	SalePoints        []SalePoint `json:"sale_points"`

	loc *time.Location
}

// Location возвращает таймзону тенанта (Europe/Moscow по умолчанию).
func (t Tenant) Location() *time.Location {
	if t.loc != nil {
		return t.loc
	}
	return time.UTC
}

// Store — справочник тенантов с поиском по tenant_id и domain.
type Store struct {
	byID     map[string]Tenant
	byDomain map[string]Tenant
}

// Load читает tenants.example.json.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("чтение %s: %w", path, err)
	}
	var doc struct {
		Tenants []Tenant `json:"tenants"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("разбор tenants: %w", err)
	}

	s := &Store{byID: map[string]Tenant{}, byDomain: map[string]Tenant{}}
	for _, t := range doc.Tenants {
		if loc, err := time.LoadLocation(t.Timezone); err == nil {
			t.loc = loc
		} else {
			t.loc = time.UTC
		}
		s.byID[t.TenantID] = t
		s.byDomain[t.Domain] = t
	}
	return s, nil
}

// Lookup ищет тенанта по идентификатору (tenant_id или domain).
// Второй результат — false, если тенант не найден.
func (s *Store) Lookup(id string) (Tenant, bool) {
	if t, ok := s.byID[id]; ok {
		return t, true
	}
	t, ok := s.byDomain[id]
	return t, ok
}
