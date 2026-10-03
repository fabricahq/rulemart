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

/** pending is an answer that never comes: the request stays open until its signal aborts it. */
const pending = Symbol('pending');

/**
 * Run poll.js on a run page that's following a check, with fetch answering each request with the next of answers, on a
 * clock that jumps to each timer as it falls due, and return the page's recovery message, the timers left waiting,
 * how many requests it sent, the waits it scheduled before each poll, and when each request was sent and aborted.
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
  let now = 0;
  let order = 0;
  // timers are the callbacks waiting on the clock: polls the script scheduled, and its requests' timeouts.
  const timers = [];
  const schedule = (fn, ms, poll) => timers.push({ due: now + ms, order: order++, fn, poll });
  const delays = [];
  const sent = [];
  const context = {
    document: page,
    location: { href: here },
    URL,
    AbortSignal: {
      timeout: (ms) => {
        const controller = new AbortController();
        schedule(() => controller.abort(new DOMException('The operation timed out.', 'TimeoutError')), ms, false);
        return controller.signal;
      },
    },
    setTimeout: (fn, ms) => {
      delays.push(ms);
      schedule(fn, ms, true);
    },
    DOMParser: class {
      parseFromString() {
        return { querySelector: (selector) => (selector === '[data-polling]' ? {} : null) };
      }
    },
    fetch: (_url, { signal } = {}) => {
      const answer = answers[Math.min(sent.length, answers.length - 1)];
      const request = { sentAt: now, abortedAt: null };
      sent.push(request);
      if (answer === pending) {
        return new Promise((_resolve, reject) => {
          signal?.addEventListener('abort', () => {
            request.abortedAt = now;
            reject(signal.reason);
          });
        });
      }
      if (answer instanceof Error) return Promise.reject(answer);
      return Promise.resolve({ ok: answer.status >= 200 && answer.status < 300, redirected: false, url: here, text: async () => '<p>checking</p>', ...answer });
    },
  };
  vm.runInNewContext(script, context);
  for (let ticks = 0; timers.length > 0 && ticks < 200; ticks++) {
    timers.sort((a, b) => a.due - b.due || a.order - b.order);
    const timer = timers.shift();
    now = timer.due;
    timer.fn();
    // Let the poll's promises settle before the clock moves on; a request still pending waits for its timeout.
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
  }
  return {
    message: stopped.hidden ? null : { text: text.textContent, link: link.textContent, href: link.href },
    waiting: timers.filter((timer) => timer.poll).length,
    requests: sent.length,
    delays,
    sent,
  };
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

test('should give up on a request that never answers after ten seconds, and try again', async () => {
  const { message, requests, sent } = await follow([pending, { status: 404 }]);

  assert.equal(sent[0].abortedAt - sent[0].sentAt, 10000);
  assert.equal(requests, 2);
  assert.match(message.text, /no longer has this listing/);
});

test('should wait longer after each failure in a row', async () => {
  const { delays } = await follow([{ status: 502 }]);

  const retries = delays.slice(1);
  assert.equal(retries.length, 4, `waited ${delays}`);
  for (let i = 1; i < retries.length; i++) {
    assert.ok(retries[i] > retries[i - 1], `waited ${delays}`);
  }
});

test('should keep following when a success comes between two runs of four failures', async () => {
  const failures = Array(4).fill({ status: 502 });

  const { message, requests } = await follow([...failures, { status: 200 }, ...failures, { status: 404 }]);

  assert.equal(requests, 10);
  assert.match(message.text, /no longer has this listing/);
});

test('should stop at the fifth failure in a row', async () => {
  const { message, requests, waiting } = await follow([{ status: 200 }, ...Array(5).fill({ status: 502 }), { status: 404 }]);

  assert.equal(requests, 6);
  assert.equal(waiting, 0);
  assert.match(message.text, /couldn't reach/);
});
