/**
 * @fileoverview Follows something that happens elsewhere, such as a library being added: every two seconds, it fetches
 * the page again and replaces each element marked data-poll with the fetched page's, until the fetched page marks
 * nothing data-polling, which means it's done. Without this script, the page reloads itself as often.
 */
(() => {
  const interval = 2000;
  if (!document.querySelector('[data-polling]')) return;

  /** Fetch the page and show what changed; keep going while it's still following. */
  async function poll() {
    let doc;
    try {
      const response = await fetch(location.href, { headers: { Accept: 'text/html' }, credentials: 'same-origin', cache: 'no-store' });
      if (!response.ok) throw new Error(`status ${response.status}`);
      doc = new DOMParser().parseFromString(await response.text(), 'text/html');
    } catch {
      setTimeout(poll, interval);
      return;
    }
    for (const target of document.querySelectorAll('[data-poll]')) {
      const fresh = doc.querySelector(`[data-poll="${target.dataset.poll}"]`);
      if (fresh && fresh.innerHTML !== target.innerHTML) target.replaceWith(document.importNode(fresh, true));
    }
    if (doc.querySelector('[data-polling]')) setTimeout(poll, interval);
  }
  setTimeout(poll, interval);
})();
