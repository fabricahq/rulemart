/** @fileoverview Applies the visitor's color theme before the page paints, and runs the footer's theme menu, the
 * Fabrica website's: a button showing the current choice, and a menu of light, dark, and system. The choice stays
 * in this browser only. System leaves the page to the prefers-color-scheme rules in the stylesheet. */
(() => {
  const key = 'rulemart-theme';
  /** Return light, dark, or system; treat an absent or unrecognized value as system. */
  const normalize = (value) => (['light', 'dark', 'system'].includes(value) ? value : 'system');
  let preference = 'system';
  try {
    preference = normalize(localStorage.getItem(key));
  } catch {
    /* Storage may be blocked: follow the system. */
  }
  /** Apply the preference to the page, and to the menu once it exists, without saving it. */
  function apply() {
    if (preference === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.dataset.theme = preference;
    const control = document.querySelector('#theme-control');
    if (!control) return;
    const label = `Color theme: ${preference[0].toUpperCase()}${preference.slice(1)}`;
    const trigger = control.querySelector('summary');
    trigger.setAttribute('aria-label', label);
    trigger.title = label;
    control.querySelectorAll('[data-theme-icon]').forEach((icon) => {
      icon.hidden = icon.dataset.themeIcon !== preference;
    });
    control.querySelectorAll('[data-theme-choice]').forEach((button) => {
      button.setAttribute('aria-pressed', String(button.dataset.themeChoice === preference));
    });
  }
  apply();
  window.addEventListener('storage', (event) => {
    if (event.key === key || event.key === null) {
      preference = normalize(event.newValue);
      apply();
    }
  });
  document.addEventListener('DOMContentLoaded', () => {
    const control = document.querySelector('#theme-control');
    if (!control) return;
    apply();
    const trigger = control.querySelector('summary');
    control.hidden = false;
    /** Close the menu and, when asked, return focus to its button. */
    const close = (restoreFocus = false) => {
      control.open = false;
      if (restoreFocus) trigger.focus();
    };
    control.querySelectorAll('[data-theme-choice]').forEach((button) => {
      button.addEventListener('click', () => {
        preference = normalize(button.dataset.themeChoice);
        try {
          localStorage.setItem(key, preference);
        } catch {
          /* Storage may be blocked: keep the choice for this page. */
        }
        apply();
        close(true);
      });
    });
    document.addEventListener('click', (event) => {
      if (!control.contains(event.target)) close();
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape' && control.open) {
        event.preventDefault();
        close(true);
      }
    });
    control.addEventListener('focusout', (event) => {
      if (!control.contains(event.relatedTarget)) close();
    });
  });
})();
