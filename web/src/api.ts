import type { AskResponse } from './types'

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api'
const TENANT_ID = 'mock_single'

/**
 * authToken — токен демо-гейта. Источник по приоритету:
 *   1) ?key= в URL страницы (открыл http://host/?key=demo123 — токен подхватился и запомнился);
 *   2) sessionStorage (сохранён с прошлого ?key=);
 *   3) VITE_AUTH_TOKEN из сборки.
 * Бэкенд принимает его заголовком X-Auth-Token. Если гейт выключен (пустой токен) — не мешает.
 */
function authToken(): string {
  try {
    const fromUrl = new URLSearchParams(window.location.search).get('key')
    if (fromUrl) {
      sessionStorage.setItem('auth_token', fromUrl)
      return fromUrl
    }
    const saved = sessionStorage.getItem('auth_token')
    if (saved) return saved
  } catch {
    // window/sessionStorage недоступны (SSR/тест) — падаем на env
  }
  return (import.meta.env.VITE_AUTH_TOKEN as string | undefined) ?? ''
}

/** Кидаем, когда бэкенд вернул не-2xx; message — текст из тела {error} либо статус. */
export class AskError extends Error {}

export async function ask(text: string, signal?: AbortSignal): Promise<AskResponse> {
  let res: Response
  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Tenant-ID': TENANT_ID,
    }
    const tok = authToken()
    if (tok) headers['X-Auth-Token'] = tok

    res = await fetch(`${API_BASE}/ask`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ text }),
      signal,
    })
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw e
    throw new AskError('Не удалось связаться с сервером. Запущен ли бэкенд на :8088?')
  }

  let data: unknown = null
  try {
    data = await res.json()
  } catch {
    // тело может быть пустым/не-JSON
  }

  if (!res.ok) {
    const msg =
      (data && typeof data === 'object' && 'error' in data && (data as { error: string }).error) ||
      `Сервер вернул ошибку ${res.status}`
    throw new AskError(String(msg))
  }

  return (data ?? {}) as AskResponse
}

/**
 * downloadExport скачивает отчёт по тому же текстовому запросу как .xlsx.
 * Бэкенд (GET /export) переигрывает тот же пайплайн и отдаёт файл; токен и тенант —
 * заголовками (не в URL), имя файла берём из Content-Disposition.
 * Кидает AskError с текстом сервера (напр. «нечего экспортировать»), если не 2xx.
 */
export async function downloadExport(text: string): Promise<void> {
  const headers: Record<string, string> = { 'X-Tenant-ID': TENANT_ID }
  const tok = authToken()
  if (tok) headers['X-Auth-Token'] = tok

  let res: Response
  try {
    res = await fetch(`${API_BASE}/export?text=${encodeURIComponent(text)}`, { headers })
  } catch {
    throw new AskError('Не удалось связаться с сервером для выгрузки.')
  }

  if (!res.ok) {
    let msg = `Сервер вернул ошибку ${res.status}`
    try {
      const d = (await res.json()) as { error?: string }
      if (d?.error) msg = d.error
    } catch {
      // тело не JSON — оставляем статус
    }
    throw new AskError(msg)
  }

  const blob = await res.blob()
  const cd = res.headers.get('Content-Disposition') ?? ''
  const m = cd.match(/filename="?([^"]+)"?/)
  const filename = m ? m[1] : 'report.xlsx'

  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
