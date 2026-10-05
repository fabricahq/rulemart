/** @fileoverview What the cart's page knows of its checkout, kept as a plain value that each function here returns
 * anew, so cart-page.js decides what to show without a browser in the way of testing it: revision, the cart's,
 * which every change to what checkout reads advances; answer, the latest answer accepted, which the page keeps showing,
 * dimmed, until the next arrives; answered, the revision that answer is for; and failed, whether the request for the
 * current revision failed. An answer or a failure counts only for the revision it was asked for, so a slow answer to an
 * older cart never replaces a newer one's, and the page offers to copy only text that matches the cart. It also owns
 * the request for an answer, post, so a test can see what the page sends. That request is the site's only one with a
 * body, so it alone sends the body's SHA-256 in x-amz-content-sha256: CloudFront's origin access control signs a
 * request's body only with that header, and the function refuses a POST whose body it didn't sign. */
(() => {
  /** Return what the page knows before it asks: revision 0, with no answer. */
  const start = () => ({ revision: 0, answer: null, answered: -1, failed: false });

  /** Return checkout after a change to the cart, or a retry: a new revision, pending, whose answer is the earlier one
   * until the new one arrives. */
  const change = (checkout) => ({ ...checkout, revision: checkout.revision + 1, failed: false });

  /** Return checkout with answer, the answer to the request for revision, when revision is still the cart's. */
  const accept = (checkout, revision, answer) =>
    (revision === checkout.revision ? { ...checkout, answer, answered: revision, failed: false } : checkout);

  /** Return checkout as failed when the request for revision failed, revision is still the cart's, and no other
   * request answered it. */
  const fail = (checkout, revision) =>
    (revision === checkout.revision && checkout.answered !== revision ? { ...checkout, failed: true } : checkout);

  /** Report whether checkout's answer is for the cart as it is now. */
  const isCurrent = (checkout) => checkout.answered === checkout.revision;

  /** Report whether checkout waits for the answer for the cart as it is now. */
  const isPending = (checkout) => !isCurrent(checkout) && !checkout.failed;

  /** Return the blocks the tab, prompt or commands, shows of checkout's answer, each copied apart: its heading, numbered,
   * or '' for a tab that shows one block, and its text. The prompt is one block; the commands are a block per step,
   * which are two when the cart holds an unvetted library: its review, then the import. No answer, or one with nothing
   * to check out, shows none. */
  function blocks(checkout, tab) {
    const { answer } = checkout;
    if (!answer) return [];
    if (tab === 'prompt') return answer.prompt ? [{ heading: '', text: answer.prompt }] : [];
    return answer.commands.map((step, i) => ({ heading: step.heading ? `${i + 1}. ${step.heading}` : '', text: step.commands }));
  }

  /** Return the lowercase hex SHA-256 of text's UTF-8 bytes, or null without Web Crypto, which browsers offer only
   * over https and on localhost. */
  async function sha256(text) {
    const subtle = globalThis.crypto?.subtle;
    if (!subtle) return null;
    const digest = await subtle.digest('SHA-256', new TextEncoder().encode(text));
    return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
  }

  /** Post cart, the choices checkout reads, to url, the cart's checkout endpoint, with the body's SHA-256 when the
   * browser can compute it, and return its answer; reject when the request fails or answers with an error. */
  async function post(url, cart) {
    const body = JSON.stringify(cart);
    const headers = { 'Content-Type': 'application/json' };
    const hash = await sha256(body);
    if (hash) headers['x-amz-content-sha256'] = hash;
    const response = await fetch(url, { method: 'POST', credentials: 'same-origin', headers, body });
    if (!response.ok) throw new Error(`checkout answered ${response.status}`);
    return response.json();
  }

  window.rulemartCheckout = { start, change, accept, fail, isCurrent, isPending, blocks, post };
})();
