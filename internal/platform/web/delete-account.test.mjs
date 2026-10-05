// Tests of static/delete-account.js, which asks for the account's login in a dialog before the Account tab's Delete my
// account posts. make check runs them with node --test when Node is installed. They live outside static/, which the
// site embeds and serves.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('./static/delete-account.js', import.meta.url), 'utf8');

/** An element that keeps its listeners, so a test can fire its events. */
function element(fields = {}) {
  const listeners = {};
  return {
    hidden: false,
    ...fields,
    addEventListener: (type, fn) => {
      (listeners[type] ??= []).push(fn);
    },
    fire(type, event = {}) {
      let prevented = false;
      for (const fn of listeners[type] ?? []) fn({ target: this, preventDefault: () => (prevented = true), ...event });
      return prevented;
    },
  };
}

/** Load delete-account.js on the Account tab of octocat's dashboard, and return the parts it acts on. */
function load() {
  const inlineField = element();
  const inline = element({ querySelector: (selector) => (selector === '[data-delete-field]' ? inlineField : null) });
  const field = element({ value: '', focused: false });
  field.focus = () => {
    field.focused = true;
  };
  const button = element({ disabled: true });
  const dialog = element({
    dataset: { login: 'octocat' },
    open: false,
    querySelector: (selector) => ({ 'input[name="login"]': field, 'button[data-delete-confirm]': button })[selector] ?? null,
    getBoundingClientRect: () => ({ left: 0, right: 100, top: 0, bottom: 100 }),
  });
  dialog.showModal = () => {
    dialog.open = true;
  };
  dialog.close = () => {
    dialog.open = false;
    dialog.fire('close');
  };
  const document = {
    querySelector: (selector) => ({ 'form[data-delete-inline]': inline, 'dialog[data-delete-dialog]': dialog })[selector] ?? null,
  };
  vm.runInNewContext(script, { document });
  return { inline, inlineField, dialog, field, button };
}

test('should hide the disclosure form\'s own login field when the dialog will ask for it', () => {
  assert.equal(load().inlineField.hidden, true);
});

test('should open the dialog, focused on its login field, instead of posting the disclosure form', () => {
  const { inline, dialog, field } = load();
  assert.equal(inline.fire('submit'), true);
  assert.equal(dialog.open, true);
  assert.equal(field.focused, true);
});

test('should enable Delete my account only while the field holds the login exactly', () => {
  const { field, button } = load();
  for (const [value, enabled] of [['octo', false], ['octocat', true], ['Octocat', false], ['octocat ', false], ['', false]]) {
    field.value = value;
    field.fire('input');
    assert.equal(button.disabled, !enabled, `typed ${JSON.stringify(value)}`);
  }
});

test('should clear the field and disable Delete my account again when the dialog closes', () => {
  const { dialog, field, button } = load();
  field.value = 'octocat';
  field.fire('input');
  dialog.close();
  assert.equal(field.value, '');
  assert.equal(button.disabled, true);
});

test('should close the dialog on a click on its backdrop, and not on one inside it', () => {
  const { inline, dialog } = load();
  inline.fire('submit');
  dialog.fire('click', { target: dialog, clientX: 50, clientY: 50 });
  assert.equal(dialog.open, true);
  dialog.fire('click', { target: dialog, clientX: 150, clientY: 50 });
  assert.equal(dialog.open, false);
});
