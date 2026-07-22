// Theme is stored in localStorage and applied as data-theme on <html>; Pico v1
// and our token layer both key off that attribute. Default is light.
export type Theme = 'light' | 'dark';

// localStorage throws in some contexts (Safari private mode, cookies disabled).
// getTheme() runs at startup and on every Layout render, so a throw here would
// break app init and the theme toggle — swallow it and fall back gracefully.
function readStored(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function getTheme(): Theme {
  const stored = readStored('theme');
  if (stored === 'dark' || stored === 'light') return stored;
  // No explicit choice yet — follow the OS preference so a dark-mode user's
  // first visit isn't a white flash. matchMedia is guarded for the test env.
  const prefersDark =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: dark)').matches;
  return prefersDark ? 'dark' : 'light';
}

export function applyStoredTheme(): void {
  document.documentElement.dataset.theme = getTheme();
}

export function setTheme(theme: Theme): void {
  try {
    localStorage.setItem('theme', theme);
  } catch {
    // Storage blocked — the toggle still applies for this session below.
  }
  document.documentElement.dataset.theme = theme;
}
