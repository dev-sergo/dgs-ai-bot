import { useState } from 'react'
import type { HistoryEntry } from '../App'
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

  if (validation?.NeedClarify) {
    const prompt = validation.ClarifyPrompt || res?.answer || 'Уточните, пожалуйста, запрос.'
    return (
      <div className="clarify fade-up">
        <span className="clarify-icon" aria-hidden="true">
          ?
        </span>
        <Typewriter text={prompt} animate={entry.animate} className="clarify-text" />
      </div>
    )
  }

  if (envelope) {
    return <EnvelopeBody entry={entry} />
  }

  if (res?.answer) {
    return <Typewriter text={res.answer} animate={entry.animate} className="plain-answer" />
  }

  return <p className="muted">Пустой ответ.</p>
}

function EnvelopeBody({ entry }: { entry: HistoryEntry }) {
  const envelope = entry.response!.envelope!
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
          {envelope.meta?.method != null && (
            <div className="meta-line">
              метод: {String(envelope.meta.method)}
              {envelope.rows?.length != null ? ` · строк: ${envelope.rows.length}` : ''}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
