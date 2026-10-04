/** @fileoverview Shows a comparison as soon as a version or release is chosen in a page's compare form, as the
 * prototype does, by submitting the form when either of its choices changes. Without JavaScript, the form shows its
 * Compare button instead. */
(() => {
  for (const form of document.querySelectorAll('form[data-compare-form]')) {
    form.addEventListener('change', (event) => {
      if (event.target instanceof HTMLSelectElement) form.requestSubmit();
    });
  }
})();
