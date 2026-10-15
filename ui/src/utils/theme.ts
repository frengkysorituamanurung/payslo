export type Theme = 'light' | 'dark';

export function applyTheme(theme: Theme): void {
  document.documentElement.dataset['theme'] = theme;
  try {
    localStorage.setItem('lab-theme', theme);
  } catch {
    // abaikan: storage penuh/diblokir, tema default tetap jalan
  }
}

export function initialTheme(): Theme {
  try {
    const saved = localStorage.getItem('lab-theme');
    if (saved === 'light' || saved === 'dark') return saved;
  } catch {
    // abaikan
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}
