/** @fileoverview What the cart's page knows of its checkout, kept as a plain value that each function here returns
 * anew, so cart-page.js decides what to show without a browser in the way of testing it: revision, the cart's,
 * which every change to what checkout reads advances; answer, the latest answer accepted, which the page keeps showing,
 * dimmed, until the next arrives; answered, the revision that answer is for; and failed, whether the request for the
 * current revision failed. An answer or a failure counts only for the revision it was asked for, so a slow answer to an
 * older cart never replaces a newer one's, and the page offers to copy only text that matches the cart. */
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

  window.rulemartCheckout = { start, change, accept, fail, isCurrent, isPending, blocks };
})();
