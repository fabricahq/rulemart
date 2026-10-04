/** @fileoverview Opens a rule page's "Sign in to star rules" dialog when a visitor who isn't signed in presses Star, as
 * the prototype does, rather than following the star's link to the sign-in page, which it still does without
 * JavaScript, or when opened in a new tab. The dialog's own buttons close it, and so does a click on its backdrop. */
(() => {
  const link = document.querySelector('a[data-star-signin]');
  const dialog = document.querySelector('dialog[data-star-dialog]');
  if (!link || !dialog) return;

  link.setAttribute('aria-haspopup', 'dialog');
  link.addEventListener('click', (event) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    dialog.showModal();
  });

  // A click on the backdrop, outside the dialog's box, closes it, as the prototype's scrim does.
  dialog.addEventListener('click', (event) => {
    if (event.target !== dialog) return;
    const box = dialog.getBoundingClientRect();
    const inside = event.clientX >= box.left && event.clientX <= box.right && event.clientY >= box.top && event.clientY <= box.bottom;
    if (!inside) dialog.close();
  });
})();
