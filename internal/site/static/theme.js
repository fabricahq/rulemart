/** @fileoverview Applies the visitor's chosen theme before the page paints, and cycles it from the footer's theme
 * button: system, light, dark. The choice stays in this browser only. */
(() => {
  const key = 'rulemart-theme';
  const next = { system: 'light', light: 'dark', dark: 'system' };
  let theme = 'system';
  try { theme = localStorage.getItem(key) || 'system'; } catch { /* storage unavailable: follow the system */ }
  if (!next[theme]) theme = 'system';
  const apply = () => {
    if (theme === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.dataset.theme = theme;
  };
  apply();
  document.addEventListener('DOMContentLoaded', () => {
    const button = document.querySelector('[data-theme-toggle]');
    if (!button) return;
    const label = () => { button.textContent = `Theme: ${theme}`; };
    label();
    button.hidden = false;
    button.addEventListener('click', () => {
      theme = next[theme];
      try { localStorage.setItem(key, theme); } catch { /* storage unavailable: keep it for this page */ }
      apply();
      label();
    });
  });
})();
