import { useState } from 'react'
import type { HistoryEntry } from '../App'
import { downloadExport, sendFeedback, AskError } from '../api'
import { headingFromSummary } from '../summary'
import { Typewriter } from './Typewriter'
import { SummaryChips } from './SummaryChips'
import { ResultTable } from './ResultTable'
import { Loader } from './Loader'

/** Одна реплика диалога: вопрос пользователя + ответ ассистента. */
export function AnswerCard({ entry }: { entry: HistoryEntry }) {
  return (
    <div className="turn">
      <div className="msg msg-user">
        <div className="bubble bubble-user">{entry.query}</div>
      </div>

      <div className="msg msg-bot">
        <div className="bot-avatar" aria-hidden="true">
          AI
        </div>
        <div className="bubble bubble-bot">
          {entry.status === 'loading' && <Loader />}
          {entry.status === 'error' && <ErrorBanner message={entry.error ?? 'Ошибка'} />}
          {entry.status === 'done' && <DoneBody entry={entry} />}
        </div>
      </div>
    </div>
  )
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="error-banner" role="alert">
      <span className="error-icon" aria-hidden="true">
        ⚠
      </span>
      <span>{message}</span>
    </div>
  )
}

function DoneBody({ entry }: { entry: HistoryEntry }) {
  const res = entry.response
  const validation = res?.validation
  const envelope = res?.envelope
  const answerID = res?.id

  if (validation?.NeedClarify) {
    const prompt = validation.ClarifyPrompt || res?.answer || 'Уточните, пожалуйста, запрос.'
    return (
      <div className="clarify fade-up">
        <span className="clarify-icon" aria-hidden="true">
          ?
        </span>
        <Typewriter text={prompt} animate={entry.animate} className="clarify-text" />
        {answerID && <FeedbackRow id={answerID} />}
      </div>
    )
  }

  if (envelope) {
    return <EnvelopeBody entry={entry} />
  }

  if (res?.answer) {
    return (
      <>
        <Typewriter text={res.answer} animate={entry.animate} className="plain-answer" />
        {answerID && <FeedbackRow id={answerID} />}
      </>
    )
  }

  return <p className="muted">Пустой ответ.</p>
}

function EnvelopeBody({ entry }: { entry: HistoryEntry }) {
  const envelope = entry.response!.envelope!
  const answerID = entry.response?.id
  const narrative = (envelope.narrative ?? '').trim()
  const heading = narrative || headingFromSummary(envelope)
  const isNarrative = Boolean(narrative)

  const [revealed, setRevealed] = useState(!entry.animate)

  return (
    <div className="envelope">
      {envelope.period && (
        <div className="period-tag">
          {envelope.period.from} — {envelope.period.to}
          {envelope.period.tz ? ` · ${envelope.period.tz}` : ''}
        </div>
      )}

      <Typewriter
        text={heading}
        animate={entry.animate}
        className={isNarrative ? 'narrative' : 'heading'}
        onDone={() => setRevealed(true)}
      />

      {revealed && (
        <div className="reveal">
          <SummaryChips envelope={envelope} />
          <ResultTable envelope={envelope} />
          {(envelope.rows?.length ?? 0) > 0 && <ExportButton query={entry.query} />}
          {envelope.meta?.method != null && (
            <div className="meta-line">
              метод: {String(envelope.meta.method)}
              {envelope.rows?.length != null ? ` · строк: ${envelope.rows.length}` : ''}
            </div>
          )}
          {answerID && <FeedbackRow id={answerID} />}
        </div>
      )}
    </div>
  )
}

/** Кнопки 👍/👎 для оценки ответа. Одноразовые — после тапа блокируются. */
function FeedbackRow({ id }: { id: string }) {
  const [voted, setVoted] = useState<'up' | 'down' | null>(null)

  async function vote(rating: 'up' | 'down') {
    if (voted) return
    setVoted(rating)
    await sendFeedback(id, rating)
  }

  return (
    <div className="feedback-row">
      <button
        className={`feedback-btn ${voted === 'up' ? 'feedback-btn--active' : ''}`}
        onClick={() => vote('up')}
        disabled={voted !== null}
        type="button"
        aria-label="Полезный ответ"
      >
        👍
      </button>
      <button
        className={`feedback-btn ${voted === 'down' ? 'feedback-btn--active' : ''}`}
        onClick={() => vote('down')}
        disabled={voted !== null}
        type="button"
        aria-label="Бесполезный ответ"
      >
        👎
      </button>
      {voted && <span className="feedback-thanks">Спасибо!</span>}
    </div>
  )
}

/** Кнопка выгрузки текущего отчёта в Excel: повторяет тот же запрос через /export. */
function ExportButton({ query }: { query: string }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handle() {
    setBusy(true)
    setError(null)
    try {
      await downloadExport(query)
    } catch (e) {
      setError(e instanceof AskError ? e.message : 'Не удалось выгрузить отчёт.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="export-row">
      <button className="export-btn" onClick={handle} disabled={busy} type="button">
        {busy ? 'Готовлю файл…' : '⤓ Скачать Excel'}
      </button>
      {error && <span className="export-error" role="alert">{error}</span>}
    </div>
  )
}
