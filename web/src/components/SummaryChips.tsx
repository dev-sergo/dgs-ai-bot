import type { Envelope } from '../types'
import { summaryChips } from '../summary'

export function SummaryChips({ envelope }: { envelope: Envelope }) {
  const chips = summaryChips(envelope)
  if (!chips.length) return null

  return (
    <div className="chips-row">
      {chips.map((chip, i) => (
        <div
          key={chip.label + i}
          className={`stat-chip tone-${chip.tone} fade-up`}
          style={{ animationDelay: `${i * 60}ms` }}
        >
          <span className="stat-label">{chip.label}</span>
          <span className="stat-value">{chip.value}</span>
        </div>
      ))}
    </div>
  )
}
