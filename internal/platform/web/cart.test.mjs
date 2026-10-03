// Tests of static/cart.js's store, window.rulemartCart, which keeps the cart in localStorage. make check runs them with
// node --test when Node is installed. They live outside static/, which the site embeds and serves.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

const STORE = 'rulemart-cart';

/** Return a localStorage that starts with entries. */
function fakeStorage(entries = {}) {
  const items = new Map(Object.entries(entries));
  return {
    getItem: (key) => (items.has(key) ? items.get(key) : null),
    setItem: (key, value) => items.set(key, String(value)),
  };
}

/** Return window.rulemartCart, as static/cart.js defines it on a page with the header's cart link and no other cart
 * control, keeping the cart in storage. */
function loadCart(storage) {
  const link = { dataset: { cartMaxItems: '100', cartMaxKeyLength: '400' } };
  const document = {
    querySelector: (selector) => (selector === '[data-cart-link]' ? link : null),
    querySelectorAll: () => [],
    addEventListener: () => {},
  };
  const window = { addEventListener: () => {}, dispatchEvent: () => {} };
  vm.runInNewContext(readFileSync(new URL('./static/cart.js', import.meta.url), 'utf8'),
    { window, document, localStorage: storage, CustomEvent, structuredClone });
  return window.rulemartCart;
}

/** Return the cart storage keeps. */
const stored = (storage) => JSON.parse(storage.getItem(STORE));

test('should stop adding the rest of a library\'s groups when turned off under a spelling other than the stored one', () => {
  // The cart chose to add the rest of the groups when the repository was Old-Owner/Rules; the catalog spells it
  // old-owner/rules now, as the cart's page's checkbox does.
  const storage = fakeStorage({
    [STORE]: JSON.stringify({ cart: ['Old-Owner/Rules::techs/go/return-errors'], restOfGroups: { 'Old-Owner/Rules': true } }),
  });
  const cart = loadCart(storage);

  cart.setRestOfGroups('old-owner/rules', false);

  assert.deepEqual(Object.keys(cart.state().restOfGroups), []);
  assert.deepEqual(Object.keys(stored(storage).restOfGroups), []);
});

test('should keep one choice per library when it is turned on under one spelling and off under another', () => {
  const cart = loadCart(fakeStorage());

  cart.setRestOfGroups('Acme/Rules', true);
  cart.setRestOfGroups('ACME/rules', false);

  assert.deepEqual(Object.keys(cart.state().restOfGroups), []);
});
