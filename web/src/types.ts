// Контракты ответа Go-сервиса POST /ask — см. docs/demo-ui-brief.md.

export type Unit = 'RUB' | 'count' | 'percent' | 'date' | 'string'

export interface Column {
  key: string
  label: string
  unit: Unit | string
}

export type Row = Record<string, string | number | null>

export interface Period {
  from: string
  to: string
  tz?: string
}

export interface Envelope {
  type: string
  period?: Period
  currency?: string
  columns: Column[]
  rows: Row[]
  summary?: Record<string, number>
  narrative?: string
  meta?: Record<string, unknown>
}

export interface Validation {
  OK: boolean
  NeedClarify: boolean
  ClarifyPrompt?: string
}

export interface AskResponse {
  id?: string
  tenant_id?: string
  plan?: unknown
  validation?: Validation
  envelope?: Envelope | null
  answer?: string
}

export interface ErrorResponse {
  error: string
}
