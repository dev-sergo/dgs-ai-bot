import { useEffect } from 'react'
import { useTypewriter } from '../useTypewriter'

interface Props {
  text: string
  /** Печатать ли эффектом (для свежего ответа). Из истории показываем сразу. */
  animate?: boolean
  className?: string
  onDone?: () => void
}

export function Typewriter({ text, animate = true, className, onDone }: Props) {
  const { shown, done } = useTypewriter(text, { enabled: animate })

  useEffect(() => {
    if (done) onDone?.()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [done])

  return (
    <p className={className}>
      {shown}
      {!done && <span className="caret" aria-hidden="true" />}
    </p>
  )
}
