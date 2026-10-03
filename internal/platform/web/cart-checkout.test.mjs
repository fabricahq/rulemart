// Tests of static/cart-checkout.js, which the cart's page keeps its checkout's state with. make check runs them with
// node --test when Node is installed. They live outside static/, which the site embeds and serves.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

/** Return window.rulemartCheckout, as static/cart-checkout.js defines it in a browser. */
function loadCheckout() {
  const window = {};
  vm.runInNewContext(readFileSync(new URL('./static/cart-checkout.js', import.meta.url), 'utf8'), { window });
  return window.rulemartCheckout;
}

const { start, change, accept, fail, isCurrent, isPending } = loadCheckout();
const first = { prompt: 'first prompt', commands: 'first commands' };
const second = { prompt: 'second prompt', commands: 'second commands' };

test('should be pending, with no answer, before the first answer arrives', () => {
  const checkout = start();

  assert.equal(isPending(checkout), true);
  assert.equal(isCurrent(checkout), false);
  assert.equal(checkout.answer, null);
});

test('should be current when the answer to the current revision arrives', () => {
  const checkout = accept(start(), start().revision, first);

  assert.equal(isCurrent(checkout), true);
  assert.equal(isPending(checkout), false);
  assert.equal(checkout.answer, first);
});

test('should be stale and pending, keeping the earlier answer to show, as soon as the cart changes', () => {
  const answered = accept(start(), 0, first);

  const checkout = change(answered);

  assert.equal(isCurrent(checkout), false);
  assert.equal(isPending(checkout), true);
  assert.equal(checkout.answer, first);
});

test('should ignore an answer to an earlier revision that arrives after the cart changed', () => {
  const asked = start();
  const changed = change(asked);

  const checkout = accept(changed, asked.revision, first);

  assert.equal(checkout.answer, null);
  assert.equal(isCurrent(checkout), false);
  assert.equal(isPending(checkout), true);
});

test('should keep the newer answer when answers arrive out of order', () => {
  const asked = start();
  const changed = change(asked);
  const answered = accept(changed, changed.revision, second);

  const checkout = accept(answered, asked.revision, first);

  assert.equal(checkout.answer, second);
  assert.equal(isCurrent(checkout), true);
});

test('should fail, and not be current, when the request for the current revision fails after an earlier answer', () => {
  const changed = change(accept(start(), 0, first));

  const checkout = fail(changed, changed.revision);

  assert.equal(checkout.failed, true);
  assert.equal(isCurrent(checkout), false);
  assert.equal(isPending(checkout), false);
  assert.equal(checkout.answer, first);
});

test('should ignore the failure of a request for an earlier revision', () => {
  const asked = start();
  const changed = change(asked);

  const checkout = fail(changed, asked.revision);

  assert.equal(checkout.failed, false);
  assert.equal(isPending(checkout), true);
});

test('should ignore a failure for a revision already answered', () => {
  const answered = accept(start(), 0, first);

  const checkout = fail(answered, 0);

  assert.equal(checkout.failed, false);
  assert.equal(isCurrent(checkout), true);
});

test('should be pending again, without the failure, when asked again after a failure', () => {
  const failed = fail(change(accept(start(), 0, first)), 1);

  const checkout = change(failed);

  assert.equal(checkout.failed, false);
  assert.equal(isPending(checkout), true);
  assert.equal(checkout.answer, first);
});
