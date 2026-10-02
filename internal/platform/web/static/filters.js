/** @fileoverview Submits a list's filters as soon as one changes, as the prototype's sidebar does, rather than waiting
 * for its Apply button, which only shows without JavaScript. Of a filter's two checkboxes marked data-exclusive, at
 * most one is on, so turning one on turns the other off. It goes to the address the server would redirect the form's
 * own submission to: without empty values, with a parameter's values joined by commas, and with the parameters in
 * order, so the page loads once. It replaces the page in the history, as the prototype does, so Back leaves the list
 * rather than undoing each choice, and the next page focuses the control that changed, with the quiet ring of a
 * control a page focuses after a click. A page the browser restores from its back-forward cache shows the controls as
 * the visitor left them, so it resets each form to its markup, which holds what the page's address chose. */
(() => {
  const focusKey = 'rulemart:filters-focus';

  /** Remembers the control that changed, and the address it leads to, for the next page to focus it. */
  const remember = (field, address) => {
    try {
      sessionStorage.setItem(focusKey, JSON.stringify({ address, name: field.name, value: field.value }));
    } catch {
      // Without storage, the next page simply doesn't focus the control.
    }
  };

  /** Focuses the control that changed on the page before, when this page is the one it led to, opening the disclosure
   * that holds it. */
  const restoreFocus = () => {
    let remembered;
    try {
      remembered = JSON.parse(sessionStorage.getItem(focusKey) || 'null');
      sessionStorage.removeItem(focusKey);
    } catch {
      return;
    }
    if (!remembered || remembered.address !== window.location.pathname + window.location.search) return;
    const field = [...document.querySelectorAll('form[data-filters] input')].find(
      (input) => input.name === remembered.name && input.value === remembered.value,
    );
    if (!field) return;
    const disclosure = field.closest('details');
    if (disclosure) disclosure.open = true;
    field.setAttribute('data-autofocused', '');
    field.focus();
  };

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
    const address = form.getAttribute('action') + (query ? `?${query}` : '');
    remember(field, address);
    window.location.replace(address);
  });
  window.addEventListener('pageshow', (event) => {
    if (!event.persisted) return;
    for (const form of document.querySelectorAll('form[data-filters]')) form.reset();
  });
  restoreFocus();
})();
