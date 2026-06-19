import { forwardRef } from 'react'

interface Props {
  value: string
  onChange: (v: string) => void
  onSubmit: () => void
  loading: boolean
  variant?: 'hero' | 'chat'
}

export const SearchBar = forwardRef<HTMLInputElement, Props>(function SearchBar(
  { value, onChange, onSubmit, loading, variant = 'hero' },
  ref,
) {
  return (
    <form
      className={`search search-${variant}`}
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit()
      }}
    >
      <span className="search-icon" aria-hidden="true">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none">
          <circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="2" />
          <path d="m20 20-3.2-3.2" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        </svg>
      </span>
      <input
        ref={ref}
        className="search-input"
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Задайте вопрос"
        autoFocus
        autoComplete="off"
      />
      <button className="search-btn" type="submit" disabled={!value.trim() || loading}>
        {loading ? '…' : '→'}
      </button>
    </form>
  )
})
