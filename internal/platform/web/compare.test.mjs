// Tests of static/compare.js, which shows a comparison as soon as a version or release is chosen. make check runs them
// with node --test when Node is installed. They live outside static/, which the site embeds and serves.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('./static/compare.js', import.meta.url), 'utf8');

class HTMLSelectElement {
  constructor(name, value) {
    this.name = name;
    this.value = value;
  }
}

/**
 * A compare form, with a select for each of choices, keyed by name, and an input that isn't a choice. Each submission
 * records the choices it sends, as the browser reads them from the form when it submits.
 */
function compareForm(choices) {
  const listeners = [];
  const selects = Object.entries(choices).map(([name, value]) => new HTMLSelectElement(name, value));
  const form = {
    selects,
    other: { name: 'note', value: '' },
    submitted: [],
    addEventListener: (type, listener) => listeners.push({ type, listener }),
    requestSubmit: () => form.submitted.push(Object.fromEntries(selects.map((s) => [s.name, s.value]))),
    /** change sets field's value, then dispatches the change event that bubbles from it to the form. */
    change: (field, value) => {
      field.value = value;
      for (const l of listeners) if (l.type === 'change') l.listener({ type: 'change', target: field });
    },
  };
  return form;
}

/** Run compare.js on a page holding forms, all of them compare forms. */
function load(...forms) {
  const page = { querySelectorAll: (selector) => (selector === 'form[data-compare-form]' ? forms : []) };
  vm.runInNewContext(script, { document: page, HTMLSelectElement });
}

test('should submit the form with the chosen versions as soon as a version is chosen', () => {
  const form = compareForm({ from: '2.1.0', to: '2.1.0' });
  load(form);

  form.change(form.selects[1], '3.0.0');

  assert.deepEqual(form.submitted, [{ from: '2.1.0', to: '3.0.0' }]);
});

test('should submit again with each new choice', () => {
  const form = compareForm({ from: '1', to: '3' });
  load(form);

  form.change(form.selects[0], '2');
  form.change(form.selects[1], '4');

  assert.deepEqual(form.submitted, [{ from: '2', to: '3' }, { from: '2', to: '4' }]);
});

test('should not submit when a field other than a choice changes', () => {
  const form = compareForm({ from: '1', to: '3' });
  load(form);

  form.change(form.other, 'anything');

  assert.deepEqual(form.submitted, []);
});

test('should submit only the form whose choice changed when a page has two', () => {
  const versions = compareForm({ from: '1.0.0', to: '2.0.0' });
  const releases = compareForm({ from: '1', to: '3' });
  load(versions, releases);

  releases.change(releases.selects[0], '2');

  assert.deepEqual(versions.submitted, []);
  assert.deepEqual(releases.submitted, [{ from: '2', to: '3' }]);
});
