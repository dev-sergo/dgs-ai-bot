import { useEffect, useRef, useState } from 'react'

/**
 * Печатает текст посимвольно. Возвращает текущую видимую часть и флаг done.
 * Скорость — символов в секунду; длинный текст печатается чуть быстрее.
 */
export function useTypewriter(full: string, opts?: { cps?: number; enabled?: boolean }) {
  const enabled = opts?.enabled ?? true
  const [shown, setShown] = useState(enabled ? '' : full)
  const [done, setDone] = useState(!enabled)
  const raf = useRef<number | null>(null)

  useEffect(() => {
    if (!enabled || !full) {
      setShown(full)
      setDone(true)
      return
    }

    setShown('')
    setDone(false)

    // Базовая скорость + ускорение для длинных нарративов.
    const cps = opts?.cps ?? Math.min(90, 45 + full.length / 12)
    let start: number | null = null
    let cancelled = false

    const step = (ts: number) => {
      if (cancelled) return
      if (start === null) start = ts
      const elapsed = (ts - start) / 1000
      const count = Math.floor(elapsed * cps)
      if (count >= full.length) {
        setShown(full)
        setDone(true)
        return
      }
      setShown(full.slice(0, count))
      raf.current = requestAnimationFrame(step)
    }

    raf.current = requestAnimationFrame(step)
    return () => {
      cancelled = true
      if (raf.current) cancelAnimationFrame(raf.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [full, enabled])

  return { shown, done }
}
