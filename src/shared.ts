export class ApiError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
  }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (typeof options.body === 'string' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const response = await fetch(path, { ...options, headers, credentials: 'same-origin' });
  if (response.status === 204) return undefined as T;
  const type = response.headers.get('content-type') ?? '';
  const data = type.includes('application/json') ? await response.json() : null;
  if (!response.ok) {
    if (response.status === 401 && path !== '/api/login' && path !== '/api/session') {
      window.dispatchEvent(new Event('biodentis:expired'));
    }
    throw new ApiError(response.status, data?.error || 'No se pudo completar la operación.');
  }
  return data as T;
}

export function escapeHtml(value: unknown): string {
  const entities: Record<string, string> = {
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  };
  return String(value ?? '').replace(/[&<>"']/g, character => entities[character]);
}

export function money(cents: number): string {
  return new Intl.NumberFormat('es-PE', {
    style: 'currency', currency: 'PEN', minimumFractionDigits: 2
  }).format(cents / 100);
}

export function localDate(): string {
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: 'America/Lima', year: 'numeric', month: '2-digit', day: '2-digit'
  }).formatToParts(new Date());
  const part = (type: string) => parts.find(item => item.type === type)?.value;
  return [part('year'), part('month'), part('day')].join('-');
}

export function dateLabel(date: string, long = false): string {
  return new Intl.DateTimeFormat('es-PE', {
    timeZone: 'UTC', weekday: long ? 'long' : undefined, day: 'numeric',
    month: long ? 'long' : 'short', year: long ? 'numeric' : undefined
  }).format(new Date(date + 'T12:00:00Z'));
}

let notificationTimer: ReturnType<typeof setTimeout>;
export function notify(message: string, type: 'success' | 'error' = 'success'): void {
  const region = document.querySelector<HTMLElement>('#notifications');
  if (!region) return;
  const toast = document.createElement('div');
  toast.className = 'toast toast-' + type;
  toast.textContent = message;
  region.replaceChildren(toast);
  clearTimeout(notificationTimer);
  notificationTimer = setTimeout(() => region.replaceChildren(), 4800);
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Ocurrió un error al completar la operación.';
}
