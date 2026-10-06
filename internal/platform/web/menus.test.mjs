// Tests of static/menus.js's part for tab bars, which scroll sideways on a phone: it scrolls the current tab into view
// as the page loads. make check runs them with node --test when Node is installed. They live outside static/, which the
// site embeds and serves.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('./static/menus.js', import.meta.url), 'utf8');

/**
 * Load menus.js on a page whose tab bar spans 16 to 374 on the screen, padded 6 pixels on each side, over a row of tabs
 * scrolled to scrollLeft, and whose current tab spans tabLeft to tabRight, and return the bar's scrollLeft once the
 * page has loaded.
 */
function load({ scrollLeft = 0, tabLeft, tabRight, scrollWidth = 400 }) {
  const bar = {
    scrollLeft,
    clientWidth: 358,
    scrollWidth,
    getBoundingClientRect: () => ({ left: 16, right: 374 }),
  };
  const current = { closest: () => bar, getBoundingClientRect: () => ({ left: tabLeft, right: tabRight }) };
  const listeners = [];
  const document = {
    addEventListener: (type, fn) => {
      if (type === 'DOMContentLoaded') listeners.push(fn);
    },
    querySelector: () => null,
    querySelectorAll: (selector) => (selector === '[data-tabs] [aria-current="page"]' ? [current] : []),
  };
  vm.runInNewContext(script, { document, getComputedStyle: () => ({ paddingLeft: '6px', paddingRight: '6px' }) });
  listeners.forEach((fn) => fn());
  return bar.scrollLeft;
}

test('should scroll the bar so the current tab ends inside its padding when the tab is past its right edge', () => {
  assert.equal(load({ tabLeft: 330, tabRight: 386 }), 18);
});

test('should scroll the bar so the current tab starts inside its padding when the tab is before its left edge', () => {
  assert.equal(load({ scrollLeft: 30, tabLeft: 6, tabRight: 90 }), 14);
});

test('should leave the bar where it is when the current tab is in view', () => {
  assert.equal(load({ tabLeft: 22, tabRight: 107 }), 0);
});

test('should leave the bar where it is when its tabs fit', () => {
  assert.equal(load({ scrollWidth: 358, tabLeft: 330, tabRight: 386 }), 0);
});
