/** @fileoverview Puts the caret at the end of the field a page focuses with autofocus, such as the listing form's after
 * a refusal, so the visitor can fix what they typed where they stopped typing. Without JavaScript, the field is still
 * focused. */
(() => {
  document.addEventListener('DOMContentLoaded', () => {
    const field = document.querySelector('input[autofocus]');
    if (field) {
      field.focus();
      field.setSelectionRange(field.value.length, field.value.length);
    }
  });
})();
