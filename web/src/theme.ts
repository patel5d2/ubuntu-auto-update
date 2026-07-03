// Theme is stored in localStorage and applied as data-theme on <html>; Pico v1
// and our token layer both key off that attribute. Default is light.
export type Theme = 'light' | 'dark';

export function getTheme(): Theme {
  return localStorage.getItem('theme') === 'dark' ? 'dark' : 'light';
}

export function applyStoredTheme(): void {
  document.documentElement.dataset.theme = getTheme();
}

export function setTheme(theme: Theme): void {
  localStorage.setItem('theme', theme);
  document.documentElement.dataset.theme = theme;
}
