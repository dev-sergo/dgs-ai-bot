import type { Column, Envelope, Row } from '../types'
import { formatValue, formatSigned, signOf } from '../format'

const NUMERIC_UNITS = new Set(['RUB', 'count', 'percent'])

function isSignedKey(key: string): boolean {
  return /delta/i.test(key)
}

function cellContent(col: Column, row: Row): { text: string; tone: 'pos' | 'neg' | 'zero' | null } {
  const raw = row[col.key]
  if (isSignedKey(col.key)) {
    return { text: formatSigned(raw, col.unit), tone: signOf(raw) }
  }
  return { text: formatValue(raw, col.unit), tone: null }
}

export function ResultTable({ envelope }: { envelope: Envelope }) {
  const columns = envelope.columns ?? []
  const rows = envelope.rows ?? []
  if (!columns.length || !rows.length) return null

  return (
    <div className="table-wrap">
      <table className="result-table">
        <thead>
          <tr>
            {columns.map((c) => (
              <th
                key={c.key}
                className={NUMERIC_UNITS.has(c.unit) ? 'num' : 'txt'}
              >
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr
              key={i}
              className="row-in"
              style={{ animationDelay: `${i * 55}ms` }}
            >
              {columns.map((c) => {
                const numeric = NUMERIC_UNITS.has(c.unit)
                const { text, tone } = cellContent(c, row)
                const cls = [
                  numeric ? 'num' : 'txt',
                  tone === 'pos' ? 'pos' : '',
                  tone === 'neg' ? 'neg' : '',
                ]
                  .filter(Boolean)
                  .join(' ')
                return (
                  <td key={c.key} className={cls}>
                    {text}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
