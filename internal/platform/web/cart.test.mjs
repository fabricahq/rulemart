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
  const cart = loadCart(fakeStorage({ [STORE]: JSON.stringify({ cart: ['acme/rules::techs/go/return-errors'] }) }));

  cart.setRestOfGroups('Acme/Rules', true);
  cart.setRestOfGroups('ACME/rules', false);

  assert.deepEqual(Object.keys(cart.state().restOfGroups), []);
});

test('should drop the choices for libraries and rules the cart no longer holds when it saves', () => {
  const storage = fakeStorage({
    [STORE]: JSON.stringify({
      cart: ['acme/rules::techs/go/return-errors', 'group::stranger/rules::techs/go'],
      fork: { 'acme/rules::techs/go/return-errors': true },
      restOfGroups: { 'acme/rules': true, 'left/rules': true },
      confirmed: { 'stranger/rules': true, 'left/rules': true },
    }),
  });
  const cart = loadCart(storage);

  cart.remove('group::stranger/rules::techs/go');

  assert.deepEqual(Object.keys(stored(storage).restOfGroups), ['acme/rules']);
  assert.deepEqual(Object.keys(stored(storage).confirmed), []);
  assert.deepEqual(Object.keys(stored(storage).fork), ['acme/rules::techs/go/return-errors']);
});

test('should drop a fork set for a rule the cart does not hold when it saves', () => {
  const storage = fakeStorage();
  const cart = loadCart(storage);

  cart.setFork('acme/rules::techs/go/return-errors', true);

  assert.deepEqual(Object.keys(stored(storage).fork), []);
});

test('should keep a confirmation given while adding the library\'s first item', () => {
  const storage = fakeStorage();
  const cart = loadCart(storage);

  cart.add(['Stranger/Rules::techs/go/use-go'], 'Stranger/Rules');

  assert.deepEqual(Object.keys(stored(storage).confirmed), ['stranger/rules']);
});

test('should keep a confirmation given on the cart\'s page for a library the cart holds', () => {
  const storage = fakeStorage({ [STORE]: JSON.stringify({ cart: ['group::Stranger/Rules::techs/go'] }) });
  const cart = loadCart(storage);

  cart.confirm('stranger/rules');

  assert.deepEqual(Object.keys(stored(storage).confirmed), ['stranger/rules']);
});

/** Return a library's Groups tab, at /example/rules, with its Add to cart box and a ticked or unticked checkbox and a
 * link to the group's page for each of groups, as static/cart.js finds them, keeping the cart in storage. It returns
 * the boxes, the links, the address the script leaves in the history, the box's parts by selector, such as its phone
 * bar, and the page's body, shows the box's own button on screen when a test calls showButton, and tells the page
 * another tab changed the cart in storage when a test calls changedElsewhere. */
function loadGroupsTab(storage, groups) {
  const parts = {};
  const panel = {
    dataset: { cartLibrary: 'example/rules', cartVetted: 'true' },
    querySelector: (selector) => (parts[selector] ??= { dataset: {}, hidden: false, offsetHeight: 61 }),
    querySelectorAll: () => [],
  };
  const boxes = groups.map(({ id, checked }) => ({
    dataset: { cartPickGroup: `group::example/rules::${id}`, cartGroupId: id, cartLibrary: 'example/rules' },
    checked,
    disabled: false,
  }));
  const links = groups.map(({ id }) => ({ dataset: { selLink: `/example/rules/${id}` }, href: `/example/rules/${id}` }));
  const found = {
    '[data-cart-groups]': [panel],
    '[data-cart-pick-group]': boxes,
    '[data-cart-pick-group][data-cart-library="example/rules"]': boxes,
    '[data-sel-link]': links,
  };
  const link = { dataset: { cartMaxItems: '100', cartMaxKeyLength: '400' } };
  const body = { style: { paddingBottom: '' } };
  const document = {
    body,
    querySelector: (selector) => (selector === '[data-cart-link]' ? link : null),
    querySelectorAll: (selector) => found[selector] || [],
    addEventListener: () => {},
  };
  const page = { address: '/example/rules?sel=unchanged' };
  const history = { state: null, replaceState: (_state, _title, address) => (page.address = address) };
  const listeners = {};
  const window = {
    addEventListener: (type, listener) => (listeners[type] ??= []).push(listener),
    dispatchEvent: () => {},
    location: { pathname: '/example/rules', hash: '' },
  };
  const observers = [];
  class IntersectionObserver {
    constructor(callback) {
      this.callback = callback;
      observers.push(this);
    }
    observe(target) {
      this.target = target;
    }
  }
  vm.runInNewContext(readFileSync(new URL('./static/cart.js', import.meta.url), 'utf8'),
    { window, document, history, localStorage: storage, CustomEvent, structuredClone, CSS: { escape: (text) => text }, IntersectionObserver });
  const showButton = (shown) => observers.forEach((o) => o.callback([{ target: o.target, isIntersecting: shown }]));
  const changedElsewhere = () => (listeners.storage || []).forEach((listener) => listener({ key: STORE }));
  return { boxes, links, page, parts, body, showButton, changedElsewhere };
}

test('should leave a group the cart holds out of the address and the group links when the page ticks it', () => {
  const storage = fakeStorage({ [STORE]: JSON.stringify({ cart: ['group::example/rules::techs/go'] }) });

  const { boxes, links, page } = loadGroupsTab(storage, [
    { id: 'techs/go', checked: true },
    { id: 'techs/react', checked: false },
    { id: 'practices/testing', checked: true },
  ]);

  assert.deepEqual(boxes.map((box) => [box.checked, box.disabled]), [[true, true], [false, false], [true, false]]);
  assert.equal(page.address, '/example/rules?sel=practices/testing');
  assert.deepEqual(links.map((link) => link.href), [
    '/example/rules/techs/go?sel=practices/testing',
    '/example/rules/techs/react?sel=practices/testing',
    '/example/rules/practices/testing?sel=practices/testing',
  ]);
});

test('should leave a group out of the address and the group links once another tab adds it to the cart', () => {
  const storage = fakeStorage();
  const { boxes, links, page, changedElsewhere } = loadGroupsTab(storage, [
    { id: 'techs/go', checked: true },
    { id: 'practices/testing', checked: true },
  ]);
  assert.equal(page.address, '/example/rules?sel=techs/go,practices/testing');

  storage.setItem(STORE, JSON.stringify({ cart: ['group::example/rules::techs/go'] }));
  changedElsewhere();

  assert.deepEqual(boxes.map((box) => [box.checked, box.disabled]), [[true, true], [true, false]]);
  assert.equal(page.address, '/example/rules?sel=practices/testing');
  assert.deepEqual(links.map((link) => link.href), [
    '/example/rules/techs/go?sel=practices/testing',
    '/example/rules/practices/testing?sel=practices/testing',
  ]);
});

test('should take the selection out of the address when nothing is ticked', () => {
  const { links, page } = loadGroupsTab(fakeStorage(), [{ id: 'techs/go', checked: false }]);

  assert.equal(page.address, '/example/rules');
  assert.deepEqual(links.map((link) => link.href), ['/example/rules/techs/go']);
});

test('should show the phone bar while groups are ticked and the box\'s own button is off screen, padding the page by it', () => {
  const { parts, body, showButton } = loadGroupsTab(fakeStorage(), [{ id: 'techs/go', checked: true }]);
  const bar = parts['[data-cart-groups-bar]'];

  assert.equal(bar.hidden, false);
  assert.equal(body.style.paddingBottom, '61px');

  showButton(true);
  assert.equal(bar.hidden, true);
  assert.equal(body.style.paddingBottom, '');

  showButton(false);
  assert.equal(bar.hidden, false);
  assert.equal(body.style.paddingBottom, '61px');
});

test('should hide the phone bar and pad nothing when no group is ticked', () => {
  const { parts, body } = loadGroupsTab(fakeStorage(), [{ id: 'techs/go', checked: false }]);

  assert.equal(parts['[data-cart-groups-bar]'].hidden, true);
  assert.equal(body.style.paddingBottom, '');
});
