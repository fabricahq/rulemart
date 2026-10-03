/** @fileoverview Submits a list's filters as soon as one changes, as the prototype's sidebar does, rather than waiting
 * for its Apply button, which only shows without JavaScript. Of a filter's two checkboxes marked data-exclusive, at
 * most one is on, so turning one on turns the other off. It goes to the address the server would redirect the form's
 * own submission to: without empty values, with a parameter's values joined by commas, with the parameters in order,
 * and spelled as the server spells them, so the page loads once. It replaces the page in the history, as the prototype
 * does, so Back leaves the list rather than undoing each choice, and the next page focuses the control that changed,
 * with the quiet ring of a control a page focuses after a click. A page the browser restores from its back-forward
 * cache shows the controls as the visitor left them, so it resets each form to its markup, which holds what the page's
 * address chose. Where the sidebar's disclosure doesn't fold and the stylesheet can't show its content closed, it opens
 * it. */
(() => {
  const focusKey = 'rulemart:filters-focus';

  /** Returns params as Go's url.Values.Encode spells them, as the server spells a list's address: URLSearchParams
   * escapes ~ and leaves * as it is, the other way around. */
  const encode = (params) => params.toString().replace(/%7E/g, '~').replace(/\*/g, '%2A');

  /** Returns a key equal for every spelling of the address, a path and query: its path and its decoded parameters,
   * sorted, so the page a redirect respells is still the one the address leads to. */
  const addressKey = (address) => {
    const compare = (a, b) => (a < b ? -1 : a > b ? 1 : 0);
    const url = new URL(address, 'https://rulemart.invalid');
    const params = [...url.searchParams].sort(([a, x], [b, y]) => compare(a, b) || compare(x, y));
    return JSON.stringify([url.pathname, params]);
  };

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
    const here = window.location.pathname + window.location.search;
    if (!remembered || addressKey(remembered.address) !== addressKey(here)) return;
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
    const query = encode(params);
    const address = form.getAttribute('action') + (query ? `?${query}` : '');
    remember(field, address);
    window.location.replace(address);
  });
  window.addEventListener('pageshow', (event) => {
    if (!event.persisted) return;
    for (const form of document.querySelectorAll('form[data-filters]')) form.reset();
  });
  // Where the sidebar doesn't fold, a browser that can't show a closed disclosure's content opens it instead.
  if (!CSS.supports('selector(::details-content)') && window.matchMedia('(width >= 45rem)').matches) {
    for (const disclosure of document.querySelectorAll('details[data-filters-disclosure]')) disclosure.open = true;
  }
  restoreFocus();
})();
