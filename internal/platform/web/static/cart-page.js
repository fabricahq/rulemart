/** @fileoverview The cart's page, /cart, which the server renders as a shell and this script fills from the cart that
 * cart.js keeps: the empty state, or the items by library, with their choices, and the prompt and commands that check
 * them out, which it asks Rulemart for each time the cart changes. It reads and changes the cart only through
 * window.rulemartCart, and shows each change, here or in another tab, when cart.js sends the rulemart:cart event. */
(() => {
  const store = window.rulemartCart;
  const page = document.querySelector('[data-cart-page]');
  if (!store || !page) return;

  const toast = (text) => window.rulemartToast?.(text);

  /** Return the plural of word for n, with n: 1 rule, 2 rules. */
  const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

  const SVG = 'http://www.w3.org/2000/svg';

  /** Return a new element of tag, with className, the attributes attrs (true for one with no value, skipped when false
   * or null), and children, each a node or text. */
  function h(tag, className, attrs = {}, ...children) {
    const el = document.createElement(tag);
    if (className) el.className = className;
    for (const [name, value] of Object.entries(attrs)) {
      if (value !== false && value != null) el.setAttribute(name, value === true ? '' : value);
    }
    el.append(...children.filter((child) => child != null && child !== false));
    return el;
  }

  /** Return an icon of the paths d, stroked in the current color, as the site's icons are. */
  function icon(className, ...paths) {
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('class', className);
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '1.7');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    for (const d of paths) {
      const path = document.createElementNS(SVG, 'path');
      path.setAttribute('d', d);
      svg.append(path);
    }
    return svg;
  }

  // A rule's document icon, the prototype's, and the warning triangle unvetted libraries show.
  const docIcon = () => icon('size-[18px]', 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z', 'M14 3v5h5M9 13h6M9 17h4');
  const warningIcon = () => icon('mt-[3px] size-[18px] shrink-0 text-warn-ink', 'M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z', 'M12 9v4M12 17h.01');

  /** Return a library owner's avatar, as the avatar part draws a small one: the image, or the owner's initial. */
  function avatar(src, owner) {
    return h('span', `relative inline-grid size-[30px] flex-none place-items-center overflow-hidden rounded-[8px] border border-border text-[11px] font-semibold text-ink ${src ? 'bg-white' : 'bg-surface-header'}`, {},
      src ? h('img', 'absolute inset-0 size-full object-cover', { src, alt: '' }) : owner.slice(0, 1).toUpperCase());
  }

  /** Return a group's tile, as the groupTile part draws the prototype's medium one: its icon, its name's initial, or,
   * for a group that isn't canonical, an empty dashed tile. */
  function groupTile(group) {
    const size = 'size-9 rounded-[9px] text-[14px]';
    if (group.icon) {
      const { src, monochrome, narrow, lightTile } = group.icon;
      return h('span', `inline-grid flex-none place-items-center border border-border-subtle ${size} ${lightTile ? 'bg-white' : 'bg-icon-tile'}`, {},
        h('img', `object-contain ${narrow ? 'size-[88%]' : 'size-[64%]'} ${monochrome ? 'icon-ink' : ''}`, { src, alt: '' }));
    }
    if (group.canonical) {
      return h('span', `inline-grid flex-none place-items-center border border-border-subtle bg-icon-tile leading-none font-semibold text-muted ${size}`, { 'aria-hidden': 'true' },
        group.name.slice(0, 1).toUpperCase());
    }
    return h('span', `inline-block flex-none border border-dashed border-border ${size}`, { 'aria-hidden': 'true' });
  }

  /** Return a list of what lists of names say: Go, Go and Testing, or Go, Testing, and React. */
  const listed = (names) => (names.length < 3 ? names.join(' and ') : `${names.slice(0, -1).join(', ')}, and ${names.at(-1)}`);

  /** Return what the cart's page says of an item checkout leaves out, or '' for one it imports. */
  function leftOutNote(item) {
    switch (item.state) {
      case 'retired': return `Retired in ${item.retiredIn}, so checkout leaves it out.`;
      case 'missing': return item.kind === 'rule' ? 'This library no longer has this rule, so checkout leaves it out.' : 'This library no longer has rules in this group, so checkout leaves it out.';
      case 'gone': return 'This library is no longer on Rulemart, so checkout leaves it out.';
      case 'unvetted': return 'Left out until you confirm its library above.';
      default: return '';
    }
  }

  const removeButton = (item) => h('button', 'grid size-7 cursor-pointer place-items-center rounded-sm text-[22px] leading-none text-muted hover:bg-surface hover:text-ink', {
    type: 'button', 'aria-label': `Remove ${item.title} from cart`, 'data-cart-drop': item.key, 'data-focus': `drop:${item.key}`,
  }, '×');

  /** Return the control of a rule that can't stay in sync on its own, which says why, and offers only Fork. */
  function forkOnly(item, why) {
    return h('div', 'flex flex-wrap items-center justify-end gap-x-2.5 gap-y-1 max-narrow:justify-start', {},
      h('span', 'text-[12.5px] text-faint', {}, why),
      h('span', 'inline-flex rounded-full border border-border p-0.5 text-[12.5px]', {},
        h('label', 'relative inline-flex cursor-pointer items-center rounded-full px-[11px] py-[3px] whitespace-nowrap text-muted has-checked:bg-surface-header has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-(--focus)', {},
          h('input', 'pointer-events-none absolute opacity-0', { type: 'checkbox', checked: item.fork, 'aria-label': `Fork ${item.title}`, 'data-cart-fork': item.key, 'data-focus': `fork:${item.key}` }),
          'Fork')));
  }

  /** Return a rule's row: its icon, title, group and version, Stay in sync or Fork, or only Fork for a rule its whole
   * group in the cart brings, or of an unvetted library, which stays at the commit reviewed, and Remove. */
  function ruleRow(item) {
    const note = leftOutNote(item);
    const title = item.href ? h('a', 'block leading-[1.35] font-semibold text-ink no-underline hover:underline', { href: item.href }, item.title)
      : h('span', 'block leading-[1.35] font-semibold', {}, item.title);
    const meta = note ? [note, item.state === 'retired' && item.href ? h('a', 'text-muted', { href: item.href }, 'See what replaced it') : null]
      : [`Rule in ${item.group.name}`, ' · ', item.version];
    let mode = h('span');
    if (item.state === 'ready' && item.inGroup) {
      mode = forkOnly(item, `Included in the ${item.group.name} group`);
    } else if (item.state === 'ready' && item.pinned) {
      mode = forkOnly(item, 'Pinned to the reviewed commit');
    } else if (item.state === 'ready') {
      const name = `mode-${item.key}`;
      const option = (value, label) => h('label', 'inline-flex cursor-pointer items-center rounded-full px-[11px] py-[3px] whitespace-nowrap text-muted has-checked:bg-surface-header has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-(--focus)', {},
        h('input', 'pointer-events-none absolute opacity-0', { type: 'radio', name, value, checked: (value === 'fork') === item.fork, 'data-cart-mode': item.key, 'data-focus': `mode:${item.key}:${value}` }),
        label);
      mode = h('div', 'relative inline-flex gap-0.5 rounded-full border border-border p-0.5 text-[12.5px]', { role: 'radiogroup', 'aria-label': `${item.title}: stay in sync or fork` },
        option('sync', 'Stay in sync'), option('fork', 'Fork'));
    }
    return h('li', 'grid grid-cols-[36px_minmax(0,1fr)_auto_28px] items-center gap-[14px] px-4 py-3 max-narrow:grid-cols-[36px_minmax(0,1fr)_28px] max-narrow:[&>:nth-child(3)]:col-start-2 max-narrow:[&>:nth-child(3)]:row-start-2 max-narrow:[&>:nth-child(3)]:justify-self-start max-narrow:[&>:nth-child(4)]:col-start-3 max-narrow:[&>:nth-child(4)]:row-start-1', {},
      h('span', 'grid size-9 place-items-center rounded-[9px] border border-border-subtle bg-paper text-muted', {}, docIcon()),
      h('div', 'min-w-0', {}, title, h('div', 'mt-[3px] flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-faint', {}, ...meta)),
      mode, removeButton(item));
  }

  // How many of a whole group's rules its row lists before it offers the rest.
  const SHOWN_RULES = 5;

  /** Return the list of the rules a whole group brings, the first SHOWN_RULES of them unless the visitor expanded it,
   * and the button that lists the rest. */
  function ruleList(item, rules) {
    if (!rules.length) return [];
    const shown = item.expanded ? rules : rules.slice(0, SHOWN_RULES);
    const list = h('ul', 'mt-2 list-disc pl-4 text-[12.5px] text-muted focus:outline-none', { tabindex: '-1', 'data-focus': `rules:${item.key}` },
      ...shown.map((r) => h('li', 'my-0.5', {}, r.title)));
    if (shown.length === rules.length) return [list];
    return [list, h('button', 'mt-1 ml-4 cursor-pointer text-[12.5px] text-muted underline underline-offset-4 hover:text-ink', {
      type: 'button', 'data-cart-more': item.key, 'aria-label': `Show the other ${rules.length - shown.length} ${item.title} rules`,
    }, `+${rules.length - shown.length} more`)];
  }

  /** Return a whole group's row, shaded: its tile, name and tag, how many rules and its ID, the rules it brings,
   * Stays in sync, or for an unvetted library's, that it's pinned, and Remove. */
  function groupRow(item) {
    const note = leftOutNote(item);
    const title = item.href ? h('a', 'leading-[1.35] font-semibold text-ink no-underline hover:underline', { href: item.href }, item.title)
      : h('span', 'leading-[1.35] font-semibold', {}, item.title);
    const rules = item.rules || [];
    return h('li', 'grid grid-cols-[36px_minmax(0,1fr)_auto_28px] items-start gap-[14px] bg-surface px-4 py-3 max-narrow:grid-cols-[36px_minmax(0,1fr)_28px] max-narrow:[&>:nth-child(3)]:col-start-2 max-narrow:[&>:nth-child(3)]:row-start-2 max-narrow:[&>:nth-child(3)]:mt-0 max-narrow:[&>:nth-child(4)]:col-start-3 max-narrow:[&>:nth-child(4)]:row-start-1', {},
      groupTile(item.group),
      h('div', 'min-w-0', {},
        h('div', 'flex flex-wrap items-center gap-2', {}, title,
          h('span', 'rounded-full border border-border px-2 text-[11px] leading-[18px] font-medium whitespace-nowrap text-muted', {}, 'Whole group')),
        h('div', 'mt-[3px] flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-faint', {},
          ...(note ? [note] : [`${count(rules.length, 'rule', 'rules')} · `, h('span', 'mono', {}, item.id)])),
        ...ruleList(item, rules)),
      h('span', 'mt-1.5 text-[13px] whitespace-nowrap text-faint', {}, item.state !== 'ready' ? '' : item.pinned ? 'Pinned to the reviewed commit' : 'Stays in sync'),
      h('span', 'mt-0.5', {}, removeButton(item)));
  }

  /** Return a library's part of the cart: its name, its items, what checkout can't import of it, and the offer to add
   * the rest of its picked rules' groups. */
  function libraryBlock(lib, items) {
    const name = lib.href ? h('a', 'font-semibold text-ink no-underline hover:underline', { href: lib.href }, lib.name) : h('b', 'font-semibold', {}, lib.name);
    const parts = [h('div', 'mb-2.5 flex flex-wrap items-center gap-2.5', {},
      avatar(lib.avatar, lib.owner), name, h('span', 'font-mono text-[12px] break-all text-faint', {}, lib.fullName),
      !lib.vetted && !lib.gone ? h('span', 'inline-flex items-center rounded-full border border-warn-border bg-warn-surface px-2 text-[11.5px] leading-[18px] font-medium text-warn-ink', {}, 'Unvetted') : null)];
    if (lib.gone) {
      parts.push(h('p', 'mb-2.5 text-[13px] text-muted', {}, 'This library is no longer on Rulemart, so checkout leaves its rules out. Remove them, or find another library.'));
    } else if (!lib.vetted && !lib.confirmed) {
      parts.push(h('div', 'mb-2.5 flex gap-3 rounded-card border border-warn-border bg-warn-surface px-4 py-3.5', { role: 'note' }, warningIcon(),
        h('div', 'min-w-0', {},
          h('p', 'text-[14px] leading-[1.5] font-semibold text-warn-ink', {}, 'This library has not been vetted. Be sure to review these rules carefully.'),
          h('p', 'mt-1 text-[13px] text-ink', {}, 'Checkout leaves its rules out until you confirm them, and then asks your agent to review them first.'),
          h('button', 'mt-2.5 inline-flex min-h-8 cursor-pointer items-center rounded-full border border-border-strong px-[13px] py-1 text-[13px] font-medium text-ink hover:border-ink hover:bg-paper', {
            type: 'button', 'data-cart-confirm-library': lib.fullName, 'data-focus': `confirm:${lib.fullName}`,
          }, 'Include its rules'))));
    }
    parts.push(h('ul', 'divide-y divide-border-subtle overflow-hidden rounded-card border border-border bg-paper', {},
      ...items.map((item) => (item.kind === 'group' ? groupRow(item) : ruleRow(item)))));
    const rest = lib.restOfGroups;
    if (rest) {
      parts.push(h('label', 'mt-2.5 flex cursor-pointer items-center gap-2 text-[13px] text-muted', {},
        h('input', 'size-4 accent-(--ink)', { type: 'checkbox', checked: rest.added, 'data-cart-rest-of-groups': lib.fullName, 'data-focus': `rest:${lib.fullName}` }),
        `Also add the other ${listed(rest.groups)} rules${rest.added ? '' : ` (${rest.rules} more)`}`));
    }
    return h('div', 'mb-[26px] last:mb-3.5', {}, ...parts);
  }

  /** Fill the cart's page, [data-cart-page], from the cart and its checkout, which it asks for each time the cart
   * changes, and keep the visitor's choices on it in the cart. */
  function cartPage(root) {
    const $ = (selector) => root.querySelector(selector);
    const list = $('[data-cart-libraries]');
    const preview = $('[data-cart-preview]');
    const repo = $('[data-cart-repo]');
    const repoNote = $('[data-cart-repo-note]');
    let tab = 'prompt';
    let answer = null;
    let failed = false;
    let asked = 0;
    let timer;
    // The keys of the whole groups whose every rule the visitor asked to see.
    const expanded = new Set();
    // The lines the preview showed last, of its tab, so lines that change stand out briefly.
    let shown = { tab: null, lines: new Set() };
    repo.value = store.state().repo;

    /** Ask for the cart's checkout, a moment after the last change, so typing asks once. */
    const request = () => {
      clearTimeout(timer);
      timer = setTimeout(checkout, answer ? 200 : 0);
    };

    /** Ask Rulemart to resolve the cart and write its texts, keeping only the latest answer. */
    async function checkout() {
      const { cart, fork, restOfGroups, confirmed, repo: repository } = store.state();
      if (!cart.length) return;
      const id = ++asked;
      try {
        const response = await fetch(root.dataset.cartCheckout, {
          method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ cart, fork, restOfGroups, confirmed, repo: repository }),
        });
        if (!response.ok) throw new Error(`checkout answered ${response.status}`);
        const body = await response.json();
        if (id !== asked) return;
        answer = body;
        failed = false;
        // Keys no page writes, such as from an older cart, name nothing to show.
        if (body.unknown.length) store.dropUnknown(body.unknown);
      } catch {
        if (id !== asked) return;
        failed = true;
      }
      show();
    }

    /** Show the cart: empty, or its cards from the latest answer, as much of it as the cart still holds. */
    function show() {
      const { cart, fork } = store.state();
      const empty = cart.length === 0;
      $('[data-cart-empty]').hidden = !empty;
      $('[data-cart-full]').hidden = empty;
      if (empty) return;
      const libraries = (answer?.libraries || [])
        .map((lib) => ({ lib, items: lib.items.filter((item) => cart.includes(item.key)).map((item) => ({ ...item, fork: !!fork[item.key], pinned: !lib.vetted, expanded: expanded.has(item.key) })) }))
        .filter(({ items }) => items.length);
      const items = libraries.flatMap(({ items: held }) => held);
      // A rule its whole group brings counts with the group.
      const rules = items.filter((item) => item.kind === 'rule' && !item.inGroup).length;
      const groups = items.filter((item) => item.kind === 'group').length;
      $('[data-cart-summary]').textContent = answer
        ? `${rules ? count(rules, 'rule', 'rules') : ''}${rules && groups ? ' and ' : ''}${groups ? count(groups, 'whole group', 'whole groups') : ''} from ${count(libraries.length, 'library', 'libraries')}`
        : '';
      const focused = document.activeElement?.dataset?.focus;
      if (answer) {
        list.replaceChildren(...libraries.map(({ lib, items: held }) => libraryBlock(lib, held)));
      } else if (failed) {
        list.replaceChildren(h('p', 'text-[14px] text-muted', {}, 'Rulemart can’t show your cart right now.'));
      }
      if (focused) root.querySelector(`[data-focus="${CSS.escape(focused)}"]`)?.focus();
      showRepository();
      showPreview();
    }

    /** Say which repository the texts name, or that what the visitor wrote names none. */
    function showRepository() {
      const written = store.state().repo.trim() !== '';
      const invalid = !!answer?.repositoryInvalid && written;
      const named = answer?.repository && written;
      repoNote.hidden = !invalid && !named;
      repoNote.className = invalid ? 'mt-2.5 border-l-2 border-ink pl-2.5 text-[13px] text-ink' : 'mt-1.5 text-[12px] text-faint';
      repoNote.replaceChildren(...(invalid ? ['That doesn’t look like a GitHub repository URL.'] : named ? ['Using ', h('span', 'mono', {}, answer.repository)] : []));
      repo.setAttribute('aria-invalid', String(invalid));
    }

    /** Show the tab's text, with each line that wasn't there last time briefly marked, and the copy button and
     * footnote that go with it. */
    function showPreview() {
      const prompt = tab === 'prompt';
      for (const button of root.querySelectorAll('[data-cart-tab]')) button.setAttribute('aria-pressed', String(button.dataset.cartTab === tab));
      preview.setAttribute('aria-label', prompt ? 'Prompt for your agent' : 'Commands to run');
      const copy = $('[data-cart-copy]');
      copy.textContent = prompt ? 'Copy prompt for agent' : 'Copy commands';
      showFootnote(prompt);
      const text = answer ? (prompt ? answer.prompt : answer.commands) : '';
      copy.disabled = !text;
      if (!text) {
        const why = failed ? 'Rulemart can’t write your checkout right now. Try again in a minute.'
          : answer ? 'Nothing in your cart can be checked out yet. Remove what checkout leaves out, or confirm its library.' : 'Writing your checkout…';
        preview.replaceChildren(h('span', 'font-sans text-[13px] text-muted', {}, why));
        shown = { tab: null, lines: new Set() };
        return;
      }
      const lines = text.split('\n');
      const before = shown.tab === tab ? shown.lines : null;
      shown = { tab, lines: new Set(lines) };
      const spans = [];
      lines.forEach((line, i) => {
        if (i) spans.push('\n');
        spans.push(h('span', before && line.trim() && !before.has(line) ? 'cart-changed' : '', {}, line || ' '));
      });
      preview.replaceChildren(...spans);
    }

    /** Say how rules move to newer versions, but not those of unvetted libraries, which are pinned to the commit
     * reviewed, and on the Commands tab, how to pin a library to a release, as the checkout's answer suggests. */
    function showFootnote(prompt) {
      const code = (text) => h('code', '', {}, text);
      const { cart, fork } = store.state();
      const pinned = (answer?.libraries || [])
        .filter((lib) => !lib.vetted && lib.items.some((item) => item.state === 'ready' && cart.includes(item.key) && !fork[item.key]))
        .map((lib) => lib.fullName);
      const parts = ['Rules move to newer versions only when your project runs ', code('code-rules project update'),
        pinned.length ? `, except those from ${listed(pinned)}, which stay at the commit you review.` : '.'];
      const pin = answer?.pin;
      if (!prompt && pin) {
        parts.push(' To pin a library to one release instead, add ', code(pin.option), ` to its add library command, ${pin.release} being ${pin.library}’s latest.`);
      }
      $('[data-cart-footnote]').replaceChildren(...parts);
    }

    /** Copy the tab's text, or select it to copy by hand when the browser refuses. */
    async function copy() {
      const text = answer ? (tab === 'prompt' ? answer.prompt : answer.commands) : '';
      if (!text) return;
      try {
        await navigator.clipboard.writeText(text);
        toast(tab === 'prompt' ? 'Prompt copied' : 'Commands copied');
      } catch {
        window.getSelection().selectAllChildren(preview);
        toast('Select the text and copy it');
      }
    }

    root.addEventListener('click', (event) => {
      const target = event.target.closest('[data-cart-drop], [data-cart-confirm-library], [data-cart-clear], [data-cart-tab], [data-cart-copy], [data-cart-more]');
      if (!target) return;
      if (target.matches('[data-cart-more]')) {
        // The button goes once the list shows every rule, so focus moves to the list.
        expanded.add(target.dataset.cartMore);
        show();
        root.querySelector(`[data-focus="${CSS.escape(`rules:${target.dataset.cartMore}`)}"]`)?.focus();
        return;
      }
      if (target.matches('[data-cart-drop]')) {
        // Focus moves to the next item's Remove, or the one before, or Clear cart.
        const drops = [...root.querySelectorAll('[data-cart-drop]')];
        const at = drops.indexOf(target);
        const next = drops[at + 1] || drops[at - 1];
        store.remove(target.dataset.cartDrop);
        toast('Removed from cart');
        (next ? root.querySelector(`[data-focus="${CSS.escape(next.dataset.focus)}"]`) : $('[data-cart-clear]'))?.focus();
        if (!store.state().cart.length) root.querySelector('[data-cart-empty] a')?.focus();
      } else if (target.matches('[data-cart-confirm-library]')) {
        store.confirm(target.dataset.cartConfirmLibrary);
      } else if (target.matches('[data-cart-clear]')) {
        store.clear();
        root.querySelector('[data-cart-empty] a')?.focus();
      } else if (target.matches('[data-cart-tab]')) {
        tab = target.dataset.cartTab;
        showPreview();
      } else if (target.matches('[data-cart-copy]')) {
        copy();
      }
    });
    root.addEventListener('change', (event) => {
      const target = event.target;
      if (target.matches('[data-cart-mode]')) {
        store.setFork(target.dataset.cartMode, target.value === 'fork');
      } else if (target.matches('[data-cart-fork]')) {
        store.setFork(target.dataset.cartFork, target.checked);
      } else if (target.matches('[data-cart-rest-of-groups]')) {
        store.setRestOfGroups(target.dataset.cartRestOfGroups, target.checked);
      }
    });
    repo.addEventListener('input', () => store.setRepo(repo.value));
    // Each change to the cart, here or in another tab, shows at once, and asks for the texts again.
    window.addEventListener('rulemart:cart', () => {
      if (document.activeElement !== repo) repo.value = store.state().repo;
      show();
      request();
    });
    show();
    request();
  }

  cartPage(page);
})();
