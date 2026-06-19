import type { Envelope } from './types'
import { formatValue, formatSigned, signOf } from './format'

// Человекочитаемые подписи и единицы для ключей summary,
// которых нет среди columns (Class B и пр.).
const KNOWN: Record<string, { label: string; unit: string; signed?: boolean }> = {
  sum_all: { label: 'Выручка', unit: 'RUB' },
  kol_vo_chekov: { label: 'Кол-во чеков', unit: 'count' },
  sredniy_chek: { label: 'Средний чек', unit: 'RUB' },
  value_now: { label: 'Текущий период', unit: 'RUB' },
  value_prev: { label: 'Предыдущий период', unit: 'RUB' },
  delta_abs: { label: 'Изменение', unit: 'RUB', signed: true },
  delta_pct: { label: 'Изменение', unit: 'percent', signed: true },
}

export interface Chip {
  label: string
  value: string
  tone: 'pos' | 'neg' | 'neutral'
}

/** Карточки-итоги (chips) из summary. Подписи берём из columns, иначе из словаря. */
export function summaryChips(env: Envelope): Chip[] {
  const summary = env.summary
  if (!summary) return []

  const byColKey = new Map(env.columns.map((c) => [c.key, c]))

  return Object.entries(summary).map(([key, raw]) => {
    const col = byColKey.get(key)
    const known = KNOWN[key]
    const label = col?.label ?? known?.label ?? key
    const unit = col?.unit ?? known?.unit ?? 'count'
    const signed = known?.signed ?? false

    const value = signed ? formatSigned(raw, unit) : formatValue(raw, unit)
    const tone = signed
      ? signOf(raw) === 'pos'
        ? 'pos'
        : signOf(raw) === 'neg'
          ? 'neg'
          : 'neutral'
      : 'neutral'

    return { label, value, tone }
  })
}

/** Короткий заголовок из summary, когда narrative пустой (Class A). */
export function headingFromSummary(env: Envelope): string {
  const s = env.summary ?? {}
  if (typeof s.sum_all === 'number') {
    return `Выручка за период: ${formatValue(s.sum_all, 'RUB')}`
  }
  const n = env.rows?.length ?? 0
  return `Готов отчёт — ${n} ${plural(n, 'строка', 'строки', 'строк')}`
}

function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10
  const m100 = n % 100
  if (m10 === 1 && m100 !== 11) return one
  if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return few
  return many
}
