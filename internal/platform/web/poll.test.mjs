// Tests of static/poll.js, which the run page follows a listing's check with. make check runs them with node --test
// when Node is installed. They live outside static/, which the site embeds and serves.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('./static/poll.js', import.meta.url), 'utf8');
const here = 'https://rulemart.test/me/add/run?repo=octocat%2Frules';

/** An element of the page: its text, whether it's hidden, and where a link leads. */
function element() {
  return { hidden: true, textContent: '', href: '' };
}

/**
 * Run poll.js on a run page that's following a check, with fetch answering each request with the next of answers, and
 * return the page's recovery message, the timers left waiting, and how many requests it sent.
 */
async function follow(answers) {
  const stopped = element();
  const text = element();
  const link = element();
  stopped.querySelector = (selector) => ({ '[data-poll-stopped-text]': text, '[data-poll-stopped-link]': link })[selector];
  const page = {
    querySelector: (selector) => ({ '[data-polling]': {}, '[data-poll-stopped]': stopped })[selector] ?? null,
    querySelectorAll: () => [],
  };
  const timers = [];
  let requests = 0;
  const context = {
    document: page,
    location: { href: here },
    URL,
    AbortSignal,
    setTimeout: (fn) => timers.push(fn),
    DOMParser: class {
      parseFromString() {
        return { querySelector: (selector) => (selector === '[data-polling]' ? {} : null) };
      }
    },
    fetch: async () => {
      const answer = answers[Math.min(requests, answers.length - 1)];
      requests++;
      if (answer instanceof Error) throw answer;
      return { ok: answer.status >= 200 && answer.status < 300, redirected: false, url: here, text: async () => '<p>checking</p>', ...answer };
    },
  };
  vm.runInNewContext(script, context);
  for (let ticks = 0; timers.length > 0 && ticks < 100; ticks++) {
    await timers.shift()();
  }
  return { message: stopped.hidden ? null : { text: text.textContent, link: link.textContent, href: link.href }, waiting: timers.length, requests };
}

test('should stop, and say the listing is gone, when the page answers 404', async () => {
  const { message, waiting, requests } = await follow([{ status: 404 }]);

  assert.equal(requests, 1);
  assert.equal(waiting, 0);
  assert.match(message.text, /no longer has this listing/);
  assert.deepEqual([message.link, message.href], ['Back to Dashboard', '/me']);
});

test('should stop, and say the listing is gone, when the page answers 410', async () => {
  const { message, waiting } = await follow([{ status: 410 }]);

  assert.equal(waiting, 0);
  assert.match(message.text, /no longer has this listing/);
});

test('should stop, and offer to sign in again, when the request is redirected to sign in', async () => {
  const signIn = 'https://rulemart.test/signin?return=%2Fme%2Fadd%2Frun%3Frepo%3Doctocat%252Frules';

  const { message, waiting, requests } = await follow([{ status: 200, redirected: true, url: signIn }]);

  assert.equal(requests, 1);
  assert.equal(waiting, 0);
  assert.match(message.text, /signed out/);
  assert.deepEqual([message.link, message.href], ['Sign in again', signIn]);
});

test('should retry a failing request a bounded number of times, then stop and offer a refresh', async () => {
  const { message, waiting, requests } = await follow([{ status: 502 }]);

  assert.ok(requests > 1 && requests <= 10, `sent ${requests} requests`);
  assert.equal(waiting, 0);
  assert.match(message.text, /couldn't reach/);
  assert.deepEqual([message.link, message.href], ['Refresh status', here]);
});

test('should retry a request that fails to send, then stop', async () => {
  const { message, waiting } = await follow([new TypeError('Failed to fetch')]);

  assert.equal(waiting, 0);
  assert.match(message.text, /couldn't reach/);
});

test('should keep following after a transient failure once a request succeeds', async () => {
  const { message, requests } = await follow([{ status: 502 }, { status: 502 }, { status: 200 }, { status: 404 }]);

  assert.equal(requests, 4);
  assert.match(message.text, /no longer has this listing/);
});
