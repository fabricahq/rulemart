/** @fileoverview Asks for the account's login in a dialog, as a final confirmation, before the Account tab's Delete my
 * account posts, with the login in the action's query. Without JavaScript, the disclosure's own form asks for it
 * instead, and leads to a page that posts. The dialog's Delete my account
 * stays disabled until the field holds the login exactly; the server, which refuses any other, is what stops a
 * deletion. Cancel, the close button, and a click on the backdrop close the dialog, which clears the field. */
(() => {
  const inline = document.querySelector('form[data-delete-inline]');
  const dialog = document.querySelector('dialog[data-delete-dialog]');
  if (!inline || !dialog) return;
  const form = dialog.querySelector('form[data-delete-form]');
  const field = form && form.querySelector('input');
  const confirm = dialog.querySelector('button[data-delete-confirm]');
  if (!form || !field || !confirm) return;
  const action = form.getAttribute('action');

  // The dialog asks for the login, so the disclosure's form needn't.
  const inlineField = inline.querySelector('[data-delete-field]');
  if (inlineField) inlineField.hidden = true;

  inline.addEventListener('submit', (event) => {
    event.preventDefault();
    dialog.showModal();
    field.focus();
  });
  // The field has no name, so the POST's body stays empty, which CloudFront requires of a request a browser can't
  // sign; the login travels in the action's query, as every input to a POST on the site does.
  field.addEventListener('input', () => {
    confirm.disabled = field.value !== dialog.dataset.login;
    form.action = `${action}?login=${encodeURIComponent(field.value)}`;
  });
  dialog.addEventListener('close', () => {
    field.value = '';
    confirm.disabled = true;
  });

  // A click on the backdrop, outside the dialog's box, closes it, as star.js's dialog does.
  dialog.addEventListener('click', (event) => {
    if (event.target !== dialog) return;
    const box = dialog.getBoundingClientRect();
    const inside = event.clientX >= box.left && event.clientX <= box.right && event.clientY >= box.top && event.clientY <= box.bottom;
    if (!inside) dialog.close();
  });
})();
