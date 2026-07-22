// Theme is stored in localStorage and applied as data-theme on <html>; Pico v1
// and our token layer both key off that attribute. Default is light.
export type Theme = 'light' | 'dark';

export function getTheme(): Theme {
  const stored = localStorage.getItem('theme');
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
  localStorage.setItem('theme', theme);
  document.documentElement.dataset.theme = theme;
}
