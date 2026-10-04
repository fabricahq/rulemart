// Tests of static/star.js, which opens a rule page's "Sign in to star rules" dialog when a visitor who isn't signed in
// presses Star. make check runs them with node --test when Node is installed. They live outside static/, which the site
// embeds and serves. The dialog's Close and Not now buttons, and its Continue link, are plain HTML, which
// stars_test.go checks.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('./static/star.js', import.meta.url), 'utf8');

/** An element that keeps its attributes and the listeners added to it, and dispatches events to them. */
function element(extra = {}) {
  const attributes = new Map();
  const listeners = [];
  return {
    attributes,
    setAttribute: (name, value) => attributes.set(name, String(value)),
    addEventListener: (type, listener) => listeners.push({ type, listener }),
    /** dispatch sends an event of type to the listeners, and returns whether one prevented its default. */
    dispatch(type, fields = {}) {
      let prevented = false;
      const event = { type, target: this, button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...fields, preventDefault: () => (prevented = true) };
      for (const l of listeners) if (l.type === type) l.listener(event);
      return prevented;
    },
    ...extra,
  };
}

/** The dialog, whose box spans 100 to 500 across and 200 to 400 down, open or closed, and modal when shown so. */
function dialog() {
  const d = element({ open: false, modal: false });
  d.showModal = () => Object.assign(d, { open: true, modal: true });
  d.close = () => Object.assign(d, { open: false, modal: false });
  d.getBoundingClientRect = () => ({ left: 100, right: 500, top: 200, bottom: 400 });
  return d;
}

/** Run star.js on a page holding link, the star's sign-in link, and box, its dialog, either of which can be absent. */
function load(link, box) {
  const found = { 'a[data-star-signin]': link, 'dialog[data-star-dialog]': box };
  vm.runInNewContext(script, { document: { querySelector: (selector) => found[selector] ?? null } });
}

test('should mark the star as opening a dialog', () => {
  const link = element();
  load(link, dialog());

  assert.equal(link.attributes.get('aria-haspopup'), 'dialog');
});

test('should open the dialog as a modal, instead of following the link, when Star is pressed', () => {
  const link = element();
  const box = dialog();
  load(link, box);

  const prevented = link.dispatch('click');

  assert.equal(prevented, true);
  assert.equal(box.open, true);
  assert.equal(box.modal, true);
});

test('should follow the link to sign in when Star is pressed with a modifier key or another button', () => {
  for (const fields of [{ metaKey: true }, { ctrlKey: true }, { shiftKey: true }, { altKey: true }, { button: 1 }]) {
    const link = element();
    const box = dialog();
    load(link, box);

    const prevented = link.dispatch('click', fields);

    assert.equal(prevented, false, JSON.stringify(fields));
    assert.equal(box.open, false, JSON.stringify(fields));
  }
});

test('should close the dialog on a click on its backdrop', () => {
  const link = element();
  const box = dialog();
  load(link, box);
  link.dispatch('click');
  assert.equal(box.open, true);

  box.dispatch('click', { clientX: 50, clientY: 300 });

  assert.equal(box.open, false);
});

test('should stay open on a click inside its box', () => {
  const link = element();
  const box = dialog();
  load(link, box);
  link.dispatch('click');

  box.dispatch('click', { clientX: 105, clientY: 205 });
  box.dispatch('click', { target: element(), clientX: 300, clientY: 300 });

  assert.equal(box.open, true);
});

test('should leave the page alone when it has no dialog, as a signed-in visitor\'s has none', () => {
  const link = element();
  load(link, null);

  assert.equal(link.attributes.size, 0);
  assert.equal(link.dispatch('click'), false);
});
