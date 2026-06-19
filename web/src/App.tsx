import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ask, AskError } from './api'
import type { AskResponse } from './types'
import { AnswerCard } from './components/AnswerCard'
import { SearchBar } from './components/SearchBar'

export interface HistoryEntry {
  id: number
  query: string
  status: 'loading' | 'done' | 'error'
  response?: AskResponse
  error?: string
  animate: boolean
}

export default function App() {
  const [input, setInput] = useState('')
  const [entries, setEntries] = useState<HistoryEntry[]>([])
  const idRef = useRef(1)
  const inputRef = useRef<HTMLInputElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const loading = entries.some((e) => e.status === 'loading')
  const hasResults = entries.length > 0

  // Автоскролл чата вниз при новых сообщениях / смене статуса.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  }, [entries])

  // Возвращаем фокус в поле после ответа.
  useEffect(() => {
    if (!loading) inputRef.current?.focus()
  }, [loading])

  async function submit(text: string) {
    const query = text.trim()
    if (!query) return

    const id = idRef.current++
    setEntries((prev) => [...prev, { id, query, status: 'loading', animate: true }])
    setInput('')

    try {
      const response = await ask(query)
      setEntries((prev) =>
        prev.map((e) => (e.id === id ? { ...e, status: 'done', response } : e)),
      )
    } catch (err) {
      const message =
        err instanceof AskError ? err.message : 'Неожиданная ошибка при запросе.'
      setEntries((prev) =>
        prev.map((e) => (e.id === id ? { ...e, status: 'error', error: message } : e)),
      )
    }
  }

  return (
    <div className="app">
      <div className="aurora" aria-hidden="true">
        <span className="blob b1" />
        <span className="blob b2" />
        <span className="blob b3" />
      </div>

      <main className={`stage ${hasResults ? 'has-results' : 'empty'}`}>
        {!hasResults ? (
          <div className="hero">
            <SearchBar
              ref={inputRef}
              value={input}
              onChange={setInput}
              onSubmit={() => submit(input)}
              loading={loading}
              variant="hero"
            />
          </div>
        ) : (
          <div className="chat">
            <div className="chat-scroll" ref={scrollRef}>
              {entries.map((entry) => (
                <AnswerCard key={entry.id} entry={entry} />
              ))}
            </div>
            <div className="chat-footer">
              <SearchBar
                ref={inputRef}
                value={input}
                onChange={setInput}
                onSubmit={() => submit(input)}
                loading={loading}
                variant="chat"
              />
            </div>
          </div>
        )}
      </main>
    </div>
  )
}
