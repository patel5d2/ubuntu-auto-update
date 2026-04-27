// Centralized API client for the Ubuntu Auto-Update web dashboard.

const API_BASE_URL = import.meta.env.VITE_API_URL || '';

function getWsBaseUrl(): string {
  const wsUrl = import.meta.env.VITE_WS_URL;
  if (wsUrl) return wsUrl;
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}`;
}

function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = localStorage.getItem('auth_token');
  if (token) headers['Authorization'] = `Bearer ${token}`;
  return headers;
}

async function request<T>(endpoint: string, init: RequestInit, redirectOn401 = true): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...init,
    headers: { ...authHeaders(), ...(init.headers || {}) },
    credentials: 'include',
  });

  if (response.status === 401) {
    if (redirectOn401) {
      localStorage.removeItem('auth_token');
      window.location.href = '/login';
    }
    throw new Error('Unauthorized');
  }

  if (!response.ok) {
    let message = `API error: ${response.status} ${response.statusText}`;
    try {
      const body = await response.json();
      if (body && typeof body.error === 'string') message = body.error;
    } catch { /* non-JSON body */ }
    throw new Error(message);
  }

  const contentType = response.headers.get('content-type');
  if (contentType && contentType.includes('application/json')) {
    return response.json();
  }
  return {} as T;
}

export function apiGet<T>(endpoint: string): Promise<T> {
  return request<T>(endpoint, { method: 'GET' });
}

export function apiPost<T>(endpoint: string, body: unknown): Promise<T> {
  return request<T>(endpoint, { method: 'POST', body: JSON.stringify(body) });
}

export async function apiLogin(username: string, password: string): Promise<{ token: string }> {
  // Login must not redirect on 401 — it would loop. Show the error to the user instead.
  const data = await request<{ token: string }>(
    '/api/v1/login',
    { method: 'POST', body: JSON.stringify({ username, password }) },
    false,
  );
  if (data.token) localStorage.setItem('auth_token', data.token);
  return data;
}

export async function apiLogout(): Promise<void> {
  try {
    await request<void>('/api/v1/logout', { method: 'POST' }, false);
  } catch {
    // network failure is non-fatal — we still clear client state below
  }
  localStorage.removeItem('auth_token');
  window.location.href = '/login';
}

export function isAuthenticated(): boolean {
  // The auth cookie is HttpOnly; we rely on the token stashed at login.
  // The server is the source of truth: a stale token returns 401 and we redirect.
  return !!localStorage.getItem('auth_token');
}

export function createWebSocket(endpoint: string): WebSocket {
  return new WebSocket(`${getWsBaseUrl()}${endpoint}`);
}
