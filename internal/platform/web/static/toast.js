/** @fileoverview Times the page's notice where the stylesheet shows it as a toast, a banner marked data-toast: it holds
 * long enough to read, longer for an info toast, which has a link to follow, then fades, as the stylesheet's
 * --toast-hold says, pausing while hovered or focused, and leaves the page. An info toast's button closes it. Without
 * JavaScript the notice stays a banner in the page. */
(() => {
  const toast = document.querySelector('[data-toast]');
  if (!toast) return;
  toast.style.setProperty('--toast-hold', `${holdFor(toast.dataset.toast, toast.textContent.trim().length)}ms`);
  toast.addEventListener('animationend', (event) => {
    if (event.animationName === 'toast-out') toast.remove();
  });
  const close = toast.querySelector('[data-toast-close]');
  if (close) {
    close.hidden = false;
    close.addEventListener('click', () => toast.remove());
  }

  /** Return how long, in milliseconds, a toast of kind with text of length characters holds before it fades: about
   * as long as reading it takes, and at least 2.2 seconds, or 6 for an info toast. */
  function holdFor(kind, length) {
    return Math.max(kind === 'info' ? 6000 : 2200, 1200 + 50 * length);
  }
})();
