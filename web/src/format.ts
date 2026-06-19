// RU-форматирование «сырых» чисел из JSON по unit колонки:
// пробел-разделитель тысяч, запятая-десятичные.

const nf2 = new Intl.NumberFormat('ru-RU', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
const nf0 = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 0 })

function asNumber(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'string' && v.trim() !== '' && !Number.isNaN(Number(v))) return Number(v)
  return null
}

/** Дата ISO `2026-06-18` → дружелюбно `18.06.2026`. Иначе как есть. */
export function formatDate(v: unknown): string {
  const s = String(v ?? '')
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(s)
  if (m) return `${m[3]}.${m[2]}.${m[1]}`
  return s
}

/** Форматирует значение ячейки по unit. */
export function formatValue(value: unknown, unit: string): string {
  if (value === null || value === undefined || value === '') return '—'

  switch (unit) {
    case 'RUB': {
      const n = asNumber(value)
      return n === null ? String(value) : `${nf2.format(n)} ₽`
    }
    case 'count': {
      const n = asNumber(value)
      return n === null ? String(value) : nf0.format(n)
    }
    case 'percent': {
      const n = asNumber(value)
      return n === null ? String(value) : `${nf2.format(n)} %`
    }
    case 'date':
      return formatDate(value)
    case 'string':
    default:
      return String(value)
  }
}

/** Значение со знаком для дельт: `-832,65 ₽`, `+1 443,87 ₽`. */
export function formatSigned(value: unknown, unit: string): string {
  const n = asNumber(value)
  if (n === null) return formatValue(value, unit)
  const sign = n > 0 ? '+' : ''
  return sign + formatValue(n, unit)
}

/** Знак числа для подсветки роста/падения. */
export function signOf(value: unknown): 'pos' | 'neg' | 'zero' {
  const n = asNumber(value)
  if (n === null || n === 0) return 'zero'
  return n > 0 ? 'pos' : 'neg'
}

export { asNumber }
