// Лоадер: пульсирующая «думающая» строка + шиммер-скелет таблицы.
export function Loader() {
  return (
    <div className="loader">
      <div className="loader-head">
        <span className="thinking-dots" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span className="loader-text">Анализирую данные…</span>
      </div>
      <div className="skeleton-table">
        <div className="sk-row sk-head" />
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="sk-row" style={{ animationDelay: `${i * 120}ms` }} />
        ))}
      </div>
    </div>
  )
}
