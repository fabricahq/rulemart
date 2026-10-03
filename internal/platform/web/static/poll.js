/**
 * @fileoverview Follows something that happens elsewhere, such as a library being added: every two seconds, it fetches
 * the page again and replaces each element marked data-poll with the fetched page's, until the fetched page marks
 * nothing data-polling, which means it's done. It stops, and shows the page's data-poll-stopped message, when the page
 * is gone (404 or 410), when the visitor is signed out (the request ends on the sign-in page), or after a few failures
 * in a row. Without this script, the page reloads itself as often.
 */
(() => {
  const interval = 2000;
  // requestTimeout bounds one request, so a request that never answers is a failure like any other.
  const requestTimeout = 10000;
  // maxFailures is how many requests in a row may fail before the page stops following.
  const maxFailures = 5;
  if (!document.querySelector('[data-polling]')) return;
  let failures = 0;

  /** Stop following, and show why, with a link that leads on: linkText to href. */
  function stop(text, linkText, href) {
    const box = document.querySelector('[data-poll-stopped]');
    if (!box) return;
    box.querySelector('[data-poll-stopped-text]').textContent = text;
    const link = box.querySelector('[data-poll-stopped-link]');
    link.textContent = linkText;
    link.href = href;
    box.hidden = false;
  }

  /** Try again after a failure, waiting longer each time, until maxFailures in a row. */
  function retry() {
    failures++;
    if (failures >= maxFailures) {
      stop("Rulemart couldn't reach the check, so this page stopped following it.", 'Refresh status', location.href);
      return;
    }
    setTimeout(poll, interval * failures);
  }

  /** Fetch the page and show what changed; keep going while it's still following. */
  async function poll() {
    let response;
    let doc;
    try {
      response = await fetch(location.href, {
        headers: { Accept: 'text/html' }, credentials: 'same-origin', cache: 'no-store', signal: AbortSignal.timeout(requestTimeout),
      });
      if (response.redirected && new URL(response.url, location.href).pathname === '/signin') {
        stop("You're signed out, so this page stopped following the check.", 'Sign in again', response.url);
        return;
      }
      if (response.status === 404 || response.status === 410) {
        stop('Rulemart no longer has this listing, so this page stopped following it. It may have been removed in another tab.', 'Back to Dashboard', '/me');
        return;
      }
      if (!response.ok) throw new Error(`status ${response.status}`);
      doc = new DOMParser().parseFromString(await response.text(), 'text/html');
    } catch {
      retry();
      return;
    }
    failures = 0;
    for (const target of document.querySelectorAll('[data-poll]')) {
      const fresh = doc.querySelector(`[data-poll="${target.dataset.poll}"]`);
      if (fresh && fresh.innerHTML !== target.innerHTML) target.replaceWith(document.importNode(fresh, true));
    }
    if (doc.querySelector('[data-polling]')) setTimeout(poll, interval);
  }
  setTimeout(poll, interval);
})();
