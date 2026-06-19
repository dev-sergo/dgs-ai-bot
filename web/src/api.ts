import type { AskResponse } from './types'

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api'
const TENANT_ID = 'mock_single'

/** Кидаем, когда бэкенд вернул не-2xx; message — текст из тела {error} либо статус. */
export class AskError extends Error {}

export async function ask(text: string, signal?: AbortSignal): Promise<AskResponse> {
  let res: Response
  try {
    res = await fetch(`${API_BASE}/ask`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Tenant-ID': TENANT_ID,
      },
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
