/** @fileoverview Submits a list's filters as soon as one changes, as the prototype's sidebar does, rather than waiting
 * for its Apply button, which only shows without JavaScript. Of a filter's two checkboxes marked data-exclusive, at
 * most one is on, so turning one on turns the other off. It goes to the address the server would redirect the form's
 * own submission to: without empty values, with a parameter's values joined by commas, and with the parameters in
 * order, so the page loads once. */
(() => {
  document.addEventListener('change', (event) => {
    const field = event.target;
    const form = field.form;
    if (!form || !form.hasAttribute('data-filters')) return;
    if (field.hasAttribute('data-exclusive') && field.checked) {
      for (const other of form.querySelectorAll(`input[data-exclusive][name="${field.name}"]`)) {
        if (other !== field) other.checked = false;
      }
    }
    const values = new Map();
    for (const [name, value] of new FormData(form)) {
      if (value === '') continue;
      values.set(name, values.has(name) ? `${values.get(name)},${value}` : value);
    }
    const params = new URLSearchParams([...values]);
    params.sort();
    const query = params.toString();
    window.location.assign(form.getAttribute('action') + (query ? `?${query}` : ''));
  });
})();
