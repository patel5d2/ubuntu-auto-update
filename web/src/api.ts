// Centralized API client for the Ubuntu Auto-Update web dashboard.
// All API calls should go through this module for consistent URL handling,
// authentication, and error management.

const API_BASE_URL = import.meta.env.VITE_API_URL || '';

// For WebSocket connections, derive from the current location
function getWsBaseUrl(): string {
  const wsUrl = import.meta.env.VITE_WS_URL;
  if (wsUrl) return wsUrl;

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}`;
}

function getAuthHeaders(): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  const token = localStorage.getItem('auth_token');
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  return headers;
}

export async function apiGet<T>(endpoint: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: 'GET',
    headers: getAuthHeaders(),
    credentials: 'include',
  });

  if (response.status === 401) {
    // Clear auth and redirect to login
    localStorage.removeItem('auth_token');
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`);
  }

  return response.json();
}

export async function apiPost<T>(endpoint: string, body: unknown): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: 'POST',
    headers: getAuthHeaders(),
    credentials: 'include',
    body: JSON.stringify(body),
  });

  if (response.status === 401) {
    localStorage.removeItem('auth_token');
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`);
  }

  // Some endpoints return no body (e.g. 201 Created, 202 Accepted)
  const contentType = response.headers.get('content-type');
  if (contentType && contentType.includes('application/json')) {
    return response.json();
  }
  return {} as T;
}

export async function apiLogin(username: string, password: string): Promise<{ token: string }> {
  const response = await fetch(`${API_BASE_URL}/api/v1/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ username, password }),
  });

  if (!response.ok) {
    throw new Error('Invalid credentials');
  }

  const data = await response.json();

  // Store the token for Bearer auth (used by API client)
  if (data.token) {
    localStorage.setItem('auth_token', data.token);
  }

  return data;
}

export function apiLogout(): void {
  localStorage.removeItem('auth_token');
  // Clear cookies by navigating to login
  window.location.href = '/login';
}

export function isAuthenticated(): boolean {
  // Check localStorage token (set by login)
  const token = localStorage.getItem('auth_token');
  if (token) return true;

  // Check for auth cookie (set by backend)
  if (document.cookie.includes('auth_token=')) return true;

  return false;
}

export function createWebSocket(endpoint: string): WebSocket {
  const baseUrl = getWsBaseUrl();
  return new WebSocket(`${baseUrl}${endpoint}`);
}
