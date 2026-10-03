/** @fileoverview Shows toasts: the page's notice, where the stylesheet draws it as one, a banner marked data-toast, and
 * those a script asks for with window.rulemartToast(text), such as the cart's, from the page's toast template. A toast
 * holds long enough to read, longer for an info toast, which has a link to follow, then fades, as the stylesheet's
 * --toast-hold says, pausing while hovered or focused, and leaves the page. An info toast's button closes it. Without
 * JavaScript the notice stays a banner in the page. */
(() => {
  /** Return how long, in milliseconds, a toast of kind with text of length characters holds before it fades: about
   * as long as reading it takes, and at least 2.2 seconds, or 6 for an info toast. */
  const holdFor = (kind, length) => Math.max(kind === 'info' ? 6000 : 2200, 1200 + 50 * length);

  /** Time toast, a banner marked data-toast on the page, and take it off the page once it fades or is closed. */
  function start(toast) {
    toast.style.setProperty('--toast-hold', `${holdFor(toast.dataset.toast, toast.textContent.trim().length)}ms`);
    toast.addEventListener('animationend', (event) => {
      if (event.animationName === 'toast-out') toast.remove();
    });
    const close = toast.querySelector('[data-toast-close]');
    if (close) {
      close.hidden = false;
      close.addEventListener('click', () => toast.remove());
    }
  }

  /** Show text as a status toast, in place of any toast showing, after the page's main content. */
  window.rulemartToast = (text) => {
    const template = document.querySelector('template[data-toast-template]');
    if (!template) return;
    document.querySelectorAll('[data-toast]').forEach((toast) => toast.remove());
    const toast = template.content.firstElementChild.cloneNode(true);
    template.before(toast);
    // Screen readers announce a status region's text that changes, not text it was added with.
    requestAnimationFrame(() => {
      toast.querySelector('[data-toast-text]').textContent = text;
      start(toast);
    });
  };

  const notice = document.querySelector('[data-toast]');
  if (notice) start(notice);
})();
