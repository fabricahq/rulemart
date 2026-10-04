/** @fileoverview The cart's page, /cart, which the server renders as a shell and this script fills from the cart that
 * cart.js keeps: the empty state, or the items by library, with their choices, and the prompt and commands that check
 * them out, which it asks Rulemart for each time the cart changes, keeping what it knows of them as cart-checkout.js
 * says. It reads and changes the cart only through window.rulemartCart, and shows each change, here or in another
 * tab, when cart.js sends the rulemart:cart event. */
(() => {
  const store = window.rulemartCart;
  const checkouts = window.rulemartCheckout;
  const page = document.querySelector('[data-cart-page]');
  if (!store || !checkouts || !page) return;

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

  /** Return a library's avatar, carrying the check mark the libraryAvatar part draws on a vetted library's, which says
   * "Vetted by Rulemart" on hover; screen readers hear it beside the library's name. */
  function libraryAvatar(lib) {
    if (!lib.vetted || lib.gone) return avatar(lib.avatar, lib.owner);
    const check = document.createElementNS(SVG, 'svg');
    check.setAttribute('viewBox', '0 0 12 12');
    check.setAttribute('aria-hidden', 'true');
    const path = document.createElementNS(SVG, 'path');
    for (const [name, value] of Object.entries({ d: 'M3.2 6.2 5.1 8l3.7-4', fill: 'none', stroke: '#fff', 'stroke-width': '1.8', 'stroke-linecap': 'round', 'stroke-linejoin': 'round' })) {
      path.setAttribute(name, value);
    }
    check.append(path);
    return h('span', 'relative inline-flex flex-none', { title: 'Vetted by Rulemart' },
      avatar(lib.avatar, lib.owner),
      h('span', 'absolute -right-[3px] -bottom-[3px] grid size-4 place-items-center rounded-full border-2 border-paper bg-vetted [&>svg]:size-2.5', { 'data-vetted': true }, check));
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

  // A block of the checkout's text, which scrolls on its own: the tab's only one, or a step of its commands, which
  // shares the height with the other step; and the Copy of a step.
  const blockStyle = 'm-0 overflow-auto rounded-[10px] border border-border bg-surface-header px-3 py-2.5 font-mono text-[11px] leading-[1.6] break-words whitespace-pre-wrap';
  const onlyBlockStyle = `${blockStyle} min-h-[120px] flex-auto wide:max-h-[360px] max-wide:max-h-[360px]`;
  const stepBlockStyle = `${blockStyle} min-h-[72px] max-h-[240px]`;
  const stepCopyStyle = 'inline-flex min-h-8 cursor-pointer items-center rounded-full border border-border-strong px-[13px] py-1 text-[13px] font-medium whitespace-nowrap text-ink hover:border-ink hover:bg-paper disabled:cursor-not-allowed disabled:opacity-45';

  /** Return text's lines as spans, each line that before, the lines shown last, didn't hold briefly marked as changed,
   * or none marked when before is null. */
  function markChanged(text, before) {
    const spans = [];
    text.split('\n').forEach((line, i) => {
      if (i) spans.push('\n');
      spans.push(h('span', before && line.trim() && !before.has(line) ? 'cart-changed' : '', {}, line || ' '));
    });
    return spans;
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

  /** Return what a rule of an unvetted library says in place of Fork: why it can't be forked, and what it is instead,
   * pinned to the commit reviewed, or included in its whole group. */
  function pinnedNote(item) {
    return h('div', 'max-w-[240px] text-right text-[12.5px] text-faint max-narrow:max-w-none max-narrow:text-left', {},
      h('span', 'block whitespace-nowrap', {}, item.inGroup ? `Included in the ${item.group.name} group` : 'Pinned to the reviewed commit'),
      h('span', 'mt-0.5 block', {}, 'Forking waits until the rules are vetted, so you get exactly the reviewed text.'));
  }

  /** Return a rule's row: its icon, title, group and version, Stay in sync or Fork, or only Fork for a rule its whole
   * group in the cart brings, or none for a rule of an unvetted library, which stays at the commit reviewed, and
   * Remove. */
  function ruleRow(item) {
    const note = leftOutNote(item);
    const title = item.href ? h('a', 'block leading-[1.35] font-semibold text-ink no-underline hover:underline', { href: item.href }, item.title)
      : h('span', 'block leading-[1.35] font-semibold', {}, item.title);
    const meta = note ? [note, item.state === 'retired' && item.href ? h('a', 'text-muted', { href: item.href }, 'See what replaced it') : null]
      : [`Rule in ${item.group.name}`, ' · ', item.version];
    let mode = h('span');
    if (item.state === 'ready' && item.pinned) {
      mode = pinnedNote(item);
    } else if (item.state === 'ready' && item.inGroup) {
      mode = forkOnly(item, `Included in the ${item.group.name} group`);
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

  /** Return the list of the first rules a whole group brings, as many as the checkout lists, and how many others it
   * brings, which link to the group's page, where they're all listed. */
  function ruleList(item) {
    const rules = item.rules || [];
    if (!rules.length) return [];
    const list = h('ul', 'mt-2 list-disc pl-4 text-[12.5px] text-muted', {}, ...rules.map((r) => h('li', 'my-0.5', {}, r.title)));
    const others = item.ruleCount - rules.length;
    if (others <= 0) return [list];
    const more = `+${others} more`;
    return [list, item.href
      ? h('a', 'mt-1 ml-4 inline-block text-[12.5px] text-muted underline underline-offset-4 hover:text-ink', {
        href: item.href, 'aria-label': `See all ${item.ruleCount} ${item.title} rules`,
      }, more)
      : h('span', 'mt-1 ml-4 inline-block text-[12.5px] text-muted', {}, more)];
  }

  /** Return a whole group's row, shaded: its tile, name and tag, how many rules and its ID, the rules it brings,
   * Stays in sync, or for an unvetted library's, that it's pinned, and Remove. */
  function groupRow(item) {
    const note = leftOutNote(item);
    const title = item.href ? h('a', 'leading-[1.35] font-semibold text-ink no-underline hover:underline', { href: item.href }, item.title)
      : h('span', 'leading-[1.35] font-semibold', {}, item.title);
    return h('li', 'grid grid-cols-[36px_minmax(0,1fr)_auto_28px] items-start gap-[14px] bg-surface px-4 py-3 max-narrow:grid-cols-[36px_minmax(0,1fr)_28px] max-narrow:[&>:nth-child(3)]:col-start-2 max-narrow:[&>:nth-child(3)]:row-start-2 max-narrow:[&>:nth-child(3)]:mt-0 max-narrow:[&>:nth-child(4)]:col-start-3 max-narrow:[&>:nth-child(4)]:row-start-1', {},
      groupTile(item.group),
      h('div', 'min-w-0', {},
        h('div', 'flex flex-wrap items-center gap-2', {}, title,
          h('span', 'rounded-full border border-border px-2 text-[11px] leading-[18px] font-medium whitespace-nowrap text-muted', {}, 'Whole group')),
        h('div', 'mt-[3px] flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-faint', {},
          ...(note ? [note] : [`${count(item.ruleCount || 0, 'rule', 'rules')} · `, h('span', 'mono', {}, item.id)])),
        ...ruleList(item)),
      h('span', 'mt-1.5 text-[13px] whitespace-nowrap text-faint', {}, item.state !== 'ready' ? '' : item.pinned ? 'Pinned to the reviewed commit' : 'Stays in sync'),
      h('span', 'mt-0.5', {}, removeButton(item)));
  }

  /** Return a library's part of the cart: its name, its items, what checkout can't import of it, and the offer to add
   * the rest of its picked rules' groups. */
  function libraryBlock(lib, items) {
    const name = lib.href ? h('a', 'font-semibold text-ink no-underline hover:underline', { href: lib.href }, lib.name) : h('b', 'font-semibold', {}, lib.name);
    const parts = [h('div', 'mb-2.5 flex flex-wrap items-center gap-2.5', {},
      libraryAvatar(lib), name, lib.vetted && !lib.gone ? h('span', 'sr-only', {}, 'Vetted by Rulemart') : null,
      h('span', 'font-mono text-[12px] break-all text-faint', {}, lib.fullName),
      !lib.vetted && !lib.gone ? h('span', 'inline-flex items-center rounded-full border border-warn-border bg-warn-surface px-2 text-[11.5px] leading-[18px] font-medium text-warn-ink', {}, 'Unvetted') : null)];
    if (lib.gone) {
      parts.push(h('p', 'mb-2.5 text-[13px] text-muted', {}, 'This library is no longer on Rulemart, so checkout leaves its rules out. Remove them, or find another library.'));
    } else if (items.some((item) => item.state === 'unvetted')) {
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
        h('input', '', { type: 'checkbox', checked: rest.added, 'data-cart-rest-of-groups': lib.fullName, 'data-focus': `rest:${lib.fullName}` }),
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
    // The signed-in visitor's projects, as radios, and the box that names a project that doesn't use Code Rules yet,
    // where the repository field is then; neither is there for a visitor Rulemart knows no project of.
    const projects = [...root.querySelectorAll('[data-cart-project]')];
    const newProject = $('[data-cart-new-project]');
    const newBox = $('[data-cart-new-box]');
    const status = $('[data-cart-status]');
    let tab = 'prompt';
    // What the page knows of the checkout: the cart's revision, and the latest answer, which shows until the next.
    let checkout = checkouts.start();
    let timer;
    // The lines the preview showed last, of its tab, so lines that change stand out briefly.
    let shown = { tab: null, lines: new Set() };
    repo.value = store.state().repo;

    /** Ask for the cart's checkout, a moment after the last change, so typing asks once. */
    const request = () => {
      clearTimeout(timer);
      timer = setTimeout(ask, checkout.answer ? 200 : 0);
    };

    /** Mark the checkout out of date, at once, and ask for the cart's, since the cart changed, or the visitor asked
     * again. */
    function changed() {
      checkout = checkouts.change(checkout);
      show();
      request();
    }

    /** Ask Rulemart to resolve the cart as it is now and write its texts, keeping the answer only while the cart stays
     * so. */
    async function ask() {
      const { cart, fork, restOfGroups, confirmed, repo: repository, project } = store.state();
      if (!cart.length) return;
      const { revision } = checkout;
      try {
        const response = await fetch(root.dataset.cartCheckout, {
          method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ cart, fork, restOfGroups, confirmed, repo: repository, project }),
        });
        if (!response.ok) throw new Error(`checkout answered ${response.status}`);
        const body = await response.json();
        const before = checkout;
        checkout = checkouts.accept(checkout, revision, body);
        if (checkout === before) return;
        // Keys no page writes, such as from an older cart, name nothing to show.
        if (body.unknown.length) store.dropUnknown(body.unknown);
      } catch {
        checkout = checkouts.fail(checkout, revision);
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
      const { answer } = checkout;
      const libraries = (answer?.libraries || [])
        .map((lib) => ({ lib, items: lib.items.filter((item) => cart.includes(item.key)).map((item) => ({ ...item, fork: lib.vetted && !!fork[item.key], pinned: !lib.vetted })) }))
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
      } else if (checkout.failed) {
        list.replaceChildren(h('p', 'text-[14px] text-muted', {}, 'Rulemart can’t show your cart right now.'));
      }
      if (focused) root.querySelector(`[data-focus="${CSS.escape(focused)}"]`)?.focus();
      showProject();
      showRepository();
      showPreview();
    }

    /** Show which of the visitor's projects checkout is for: the one the cart names, or the first, or with the new
     * project's box open, none, for one that doesn't use Code Rules yet. */
    function showProject() {
      if (!projects.length) return;
      const { project } = store.state();
      const isNew = project === 'new';
      const chosen = projects.find((radio) => radio.value === project) || projects[0];
      for (const radio of projects) radio.checked = !isNew && radio === chosen;
      newBox.hidden = !isNew;
      newProject.hidden = isNew;
    }

    /** Say which repository the texts name, or that what the visitor wrote names none. */
    function showRepository() {
      const { answer } = checkout;
      // A known project names its own repository; the field is for one Rulemart doesn't know.
      const written = store.state().repo.trim() !== '' && answer?.mode !== 'known';
      const invalid = !!answer?.repositoryInvalid && written;
      const named = answer?.repository && written;
      repoNote.hidden = !invalid && !named;
      repoNote.className = invalid ? 'mt-2.5 border-l-2 border-ink pl-2.5 text-[13px] text-ink' : 'mt-1.5 text-[12px] text-faint';
      repoNote.replaceChildren(...(invalid ? ['That doesn’t look like a GitHub repository URL.'] : named ? ['Using ', h('span', 'mono', {}, answer.repository)] : []));
      repo.setAttribute('aria-invalid', String(invalid));
    }

    /** Show the tab's text, dimmed while it's out of date, with each line that wasn't there last time briefly marked,
     * whether it's out of date, and the copy buttons, which copy only text that matches the cart, and footnote that go
     * with it. The text is one block, with the Copy below it, or the commands' steps, which review an unvetted library
     * before importing it, each under its heading with a Copy of its own, so no one copies the import with the review. */
    function showPreview() {
      const prompt = tab === 'prompt';
      for (const button of root.querySelectorAll('[data-cart-tab]')) button.setAttribute('aria-pressed', String(button.dataset.cartTab === tab));
      preview.setAttribute('aria-label', prompt ? 'Prompt for your agent' : 'Commands to run');
      const copy = $('[data-cart-copy]');
      copy.textContent = prompt ? 'Copy prompt for agent' : 'Copy commands';
      showFootnote(prompt);
      showStatus(prompt);
      const { answer } = checkout;
      const current = checkouts.isCurrent(checkout);
      const blocks = checkouts.blocks(checkout, tab);
      copy.disabled = !current || !blocks.length;
      copy.hidden = blocks.length > 1;
      preview.classList.toggle('opacity-50', !!answer && !current);
      preview.setAttribute('aria-busy', String(checkouts.isPending(checkout)));
      if (!blocks.length) {
        const why = answer ? 'Nothing in your cart can be checked out yet. Remove what checkout leaves out, or confirm its library.'
          : checkout.failed ? '' : 'Writing your checkout…';
        preview.replaceChildren(h('pre', onlyBlockStyle, {}, h('span', 'font-sans text-[13px] text-muted', {}, why)));
        shown = { tab: null, lines: new Set() };
        return;
      }
      const focused = preview.contains(document.activeElement) ? document.activeElement.dataset.focus : null;
      const before = shown.tab === tab ? shown.lines : null;
      shown = { tab, lines: new Set(blocks.flatMap(({ text }) => text.split('\n'))) };
      preview.replaceChildren(...blocks.map(({ heading, text }, i) => {
        const pre = h('pre', heading ? stepBlockStyle : onlyBlockStyle, { tabindex: '0', 'aria-label': heading || preview.getAttribute('aria-label') },
          ...markChanged(text, before));
        if (!heading) return pre;
        return h('div', 'flex min-h-0 flex-col gap-1.5', {},
          h('div', 'flex items-center justify-between gap-3', {},
            h('h3', 'text-[13px] font-semibold text-ink', {}, heading),
            h('button', stepCopyStyle, {
              type: 'button', disabled: !current, 'data-cart-copy-step': String(i), 'data-focus': `copy:${i}`,
              'aria-label': `Copy the commands of step ${i + 1}`,
            }, 'Copy')),
          pre);
      }));
      if (focused) preview.querySelector(`[data-focus="${CSS.escape(focused)}"]`)?.focus();
    }

    /** Say that the tab's text is out of date: updating, once the page has an answer to show meanwhile, or that
     * updating failed, with a button to ask again. */
    function showStatus(prompt) {
      const failed = checkout.failed;
      status.className = failed ? 'mt-2.5 border-l-2 border-ink pl-2.5 text-[13px] text-ink' : 'mt-2.5 text-[13px] text-muted empty:mt-0';
      if (failed) {
        status.replaceChildren(`Couldn’t update the ${prompt ? 'prompt' : 'commands'}. `,
          h('button', 'cursor-pointer underline underline-offset-4 hover:text-muted', { type: 'button', 'data-cart-retry': true, 'data-focus': 'retry' }, 'Try again'));
      } else {
        status.replaceChildren(checkout.answer && checkouts.isPending(checkout) ? 'Updating…' : '');
      }
    }

    /** Say how rules move to newer versions, but not those of unvetted libraries, which are pinned to the commit
     * reviewed, and on the Commands tab, how to pin a library to a release, as the checkout's answer suggests. */
    function showFootnote(prompt) {
      const code = (text) => h('code', '', {}, text);
      const { cart } = store.state();
      const { answer } = checkout;
      const pinned = (answer?.libraries || [])
        .filter((lib) => !lib.vetted && lib.items.some((item) => item.state === 'ready' && cart.includes(item.key)))
        .map((lib) => lib.fullName);
      const parts = ['Rules move to newer versions only when your project runs ', code('code-rules project update'),
        pinned.length ? `, except those from ${listed(pinned)}, which stay at the commit you review.` : '.'];
      const pin = answer?.pin;
      if (!prompt && pin) {
        parts.push(' To pin a library to one release instead, add ', code(pin.option), ` to its add library command; ${pin.release} is the latest release of ${pin.library}.`);
      }
      $('[data-cart-footnote]').replaceChildren(...parts);
    }

    /** Copy the tab's block at, or select it to copy by hand when the browser refuses. */
    async function copy(at) {
      const block = checkouts.isCurrent(checkout) ? checkouts.blocks(checkout, tab)[at] : null;
      if (!block) return;
      try {
        await navigator.clipboard.writeText(block.text);
        toast(tab === 'prompt' ? 'Prompt copied' : 'Commands copied');
      } catch {
        window.getSelection().selectAllChildren(preview.querySelectorAll('pre')[at]);
        toast('Select the text and copy it');
      }
    }

    root.addEventListener('click', (event) => {
      const target = event.target.closest('[data-cart-drop], [data-cart-confirm-library], [data-cart-clear], [data-cart-tab], [data-cart-copy], [data-cart-copy-step], [data-cart-retry], [data-cart-new-project], [data-cart-existing]');
      if (!target) return;
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
        copy(0);
      } else if (target.matches('[data-cart-copy-step]')) {
        copy(Number(target.dataset.cartCopyStep));
      } else if (target.matches('[data-cart-new-project]')) {
        store.setProject('new');
        repo.focus();
      } else if (target.matches('[data-cart-existing]')) {
        store.setProject('');
        (projects.find((radio) => radio.checked) || projects[0])?.focus();
      } else if (target.matches('[data-cart-retry]')) {
        // The button goes once the page asks again, so focus moves to the text it updates.
        changed();
        preview.focus();
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
      } else if (target.matches('[data-cart-project]')) {
        store.setProject(target.value);
      }
    });
    repo.addEventListener('input', () => store.setRepo(repo.value));
    // Each change to the cart, here or in another tab, shows at once, out of date, and asks for the texts again.
    window.addEventListener('rulemart:cart', () => {
      if (document.activeElement !== repo) repo.value = store.state().repo;
      changed();
    });
    show();
    request();
  }

  cartPage(page);
})();
