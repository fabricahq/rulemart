/** @fileoverview The cart, which lives in the visitor's browser, as the prototype's does, so it needs no account and
 * the pages stay the same for everyone. It keeps localStorage's rulemart-cart: cart, the ordered keys of whole groups,
 * group::owner/repo::kind/group, and rules, owner/repo::kind/group/slug, at most 100; fork, the rules the visitor
 * forks; full, the libraries whose picked rules' groups they add whole; project and repo, where checkout's texts go;
 * and confirmed, the unvetted libraries they confirmed adding from. It paints the header's count and each page's cart
 * controls from the data attributes the page renders, opens the dialogs that add, and toasts what changed. Without
 * JavaScript, or storage, there's no cart: the stylesheet hides every control marked data-needs-script. */
(() => {
  const STORE = 'rulemart-cart';
  const MAX_ITEMS = 100;
  const MAX_KEY = 400;
  // A key as the server's ParseCartKey reads it: a library's owner and name, then a group's ID, two parts, or a
  // rule's, three or more, each part as Code Rules spells IDs.
  const NAME = '[A-Za-z0-9_.-]+';
  const PART = '[A-Za-z0-9][A-Za-z0-9._-]*';
  const RULE_KEY = new RegExp(`^${NAME}/${NAME}::${PART}(?:/${PART}){2,}$`);
  const GROUP_KEY = new RegExp(`^group::${NAME}/${NAME}::${PART}/${PART}$`);

  /** Report whether key is one the cart can hold. */
  const validKey = (key) => typeof key === 'string' && key.length <= MAX_KEY && (RULE_KEY.test(key) || GROUP_KEY.test(key));

  /** Return the library of a cart key, as owner/repo. */
  const libraryOf = (key) => key.replace(/^group::/, '').split('::')[0];

  /** Return the plural of word for n, with n: 1 rule, 2 rules. */
  const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

  const fresh = () => ({ cart: [], fork: {}, full: {}, project: null, repo: '', confirmed: {} });

  /** Return the names of value, an object of flags, that are on, at most MAX_ITEMS of them, as an object of flags. */
  function flags(value) {
    const out = {};
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      for (const [name, on] of Object.entries(value).slice(0, MAX_ITEMS)) {
        if (on === true && name.length <= MAX_KEY) out[name] = true;
      }
    }
    return out;
  }

  /** Read the cart from localStorage, keeping only what a cart can hold: well-formed keys, each once, at most
   * MAX_ITEMS, and choices of the right types. Storage that's empty, refused, or holds anything else is an empty cart. */
  function load() {
    const state = fresh();
    let stored;
    try {
      stored = JSON.parse(localStorage.getItem(STORE) || '{}');
    } catch {
      return state;
    }
    if (!stored || typeof stored !== 'object') return state;
    if (Array.isArray(stored.cart)) state.cart = [...new Set(stored.cart.filter(validKey))].slice(0, MAX_ITEMS);
    state.fork = Object.fromEntries(Object.keys(flags(stored.fork)).filter((key) => state.cart.includes(key)).map((key) => [key, true]));
    state.full = flags(stored.full);
    state.confirmed = flags(stored.confirmed);
    if (typeof stored.project === 'string') state.project = stored.project.slice(0, MAX_KEY);
    if (typeof stored.repo === 'string') state.repo = stored.repo.slice(0, 500);
    return state;
  }

  let state = load();

  /** Keep the cart, and paint every page part that shows it. A browser that refuses storage keeps it for this page. */
  function save() {
    try {
      localStorage.setItem(STORE, JSON.stringify(state));
    } catch {
      // Storage is full or refused: the cart lasts as long as this page.
    }
    paint();
  }

  const inCart = (key) => !!key && state.cart.includes(key);
  const toast = (text) => window.rulemartToast?.(text);

  /** Add keys the cart doesn't hold, in order, while it has room, and return how many it added, saying so when the
   * cart is full. Keys it holds already count as added, since the visitor sees them in it. */
  function add(keys) {
    const adding = keys.filter((key) => validKey(key) && !inCart(key));
    const room = Math.max(MAX_ITEMS - state.cart.length, 0);
    state.cart.push(...adding.slice(0, room));
    save();
    if (adding.length > room) {
      toast(`Your cart holds ${MAX_ITEMS} items, as many as it can. Remove some, or add a whole group.`);
    }
    return keys.length - adding.length + Math.min(adding.length, room);
  }

  /** Remove key from the cart, with its fork. */
  function remove(key) {
    state.cart = state.cart.filter((held) => held !== key);
    delete state.fork[key];
    save();
  }

  /** Paint the header's count and every cart control on the page from the cart. */
  function paint() {
    paintCount();
    paintControls();
    paintGroups();
    window.dispatchEvent(new CustomEvent('rulemart:cart'));
  }

  /** Paint the header's cart link: the count of items, a whole group counting once, in a badge, and in its name. */
  function paintCount() {
    const n = state.cart.length;
    for (const link of document.querySelectorAll('[data-cart-link]')) {
      link.setAttribute('aria-label', n ? `Cart, ${count(n, 'item', 'items')}` : 'Cart');
      const badge = link.querySelector('[data-cart-count]');
      badge.textContent = String(n);
      badge.hidden = n === 0;
    }
  }

  /** Paint each control that adds a rule or a group: Add, or once the cart holds its item, or a rule's whole group,
   * In cart, Checkout, and Remove, which names the group when it's the group the cart holds. */
  function paintControls() {
    for (const control of document.querySelectorAll('[data-cart-control]')) {
      const { cartRule: rule, cartGroup: group, cartGroupName: groupName } = control.dataset;
      const held = inCart(rule) ? rule : inCart(group) ? group : '';
      const asGroup = !!rule && held === group;
      control.querySelector('[data-cart-open], [data-cart-add-group]').hidden = !!held;
      const box = control.querySelector('[data-cart-held]');
      box.hidden = !held;
      box.querySelector('[data-cart-held-text]').textContent = asGroup ? `${groupName} group in cart` : 'In cart';
      const removeButton = box.querySelector('[data-cart-remove]');
      removeButton.textContent = asGroup ? `Remove ${groupName} group from cart` : 'Remove from cart';
      removeButton.dataset.key = held;
    }
    for (const badge of document.querySelectorAll('[data-cart-held-badge]')) {
      badge.hidden = !inCart(badge.dataset.cartHeldBadge);
    }
  }

  /** Paint the library page's Add to cart box from the groups ticked: how many, and its button's label. */
  function paintGroups() {
    const panel = document.querySelector('[data-cart-groups]');
    if (!panel) return;
    const n = document.querySelectorAll('[data-cart-pick-group]:checked').length;
    panel.querySelector('[data-cart-groups-text]').textContent = n
      ? `${count(n, 'group', 'groups')} selected. Whole groups stay in sync with ${panel.dataset.cartLibrary}.`
      : 'Select whole groups to add. You can also add single rules from their pages.';
    panel.querySelector('[data-cart-groups-label]').textContent = n ? `Add ${count(n, 'group', 'groups')} to cart` : 'Add groups to cart';
    panel.querySelector('[data-cart-groups-add]').disabled = n === 0;
    panel.querySelector('[data-cart-groups-clear]').hidden = n === 0;
  }

  const dialog = document.querySelector('dialog[data-cart-dialog]');
  // What a confirmation from an unvetted library goes on to do: show the dialog's choices, or add what the visitor
  // asked to.
  let afterConfirming = null;

  /** Show the dialog's step, confirm or choose, and hide the other. */
  function showStep(step) {
    for (const part of dialog.querySelectorAll('[data-cart-step]')) part.hidden = part.dataset.cartStep !== step;
  }

  /** Run then, after the visitor confirms adding from library when the control that asks, of data, says it isn't
   * vetted; at once otherwise. */
  function confirmed(data, then) {
    if (data.cartVetted !== 'false' || !dialog) {
      then();
      return;
    }
    afterConfirming = () => {
      dialog.close();
      then();
    };
    showStep('confirm');
    dialog.showModal();
  }

  /** Focus the Checkout link of control, once it shows the cart holds its item. */
  const focusCheckout = (control) => control.querySelector('[data-cart-held] a')?.focus();

  document.addEventListener('click', (event) => {
    const target = event.target.closest(
      '[data-cart-open], [data-cart-pick], [data-cart-confirm], [data-cart-close], [data-cart-remove], [data-cart-add-group], [data-cart-groups-add], [data-cart-groups-all], [data-cart-groups-clear]',
    );
    if (!target) return;
    const control = target.closest('[data-cart-control]') || document.querySelector('[data-cart-control]');
    if (target.matches('[data-cart-open]')) {
      // A rule page's dialog: for an unvetted library, the warning first, then the choices.
      afterConfirming = () => showStep('choose');
      showStep(control.dataset.cartVetted === 'false' ? 'confirm' : 'choose');
      dialog.showModal();
    } else if (target.matches('[data-cart-confirm]')) {
      const data = (document.querySelector('[data-cart-control]') || document.querySelector('[data-cart-groups]')).dataset;
      state.confirmed[data.cartLibrary] = true;
      save();
      afterConfirming?.();
    } else if (target.matches('[data-cart-close]')) {
      dialog.close();
    } else if (target.matches('[data-cart-pick]')) {
      dialog.close();
      if (add([target.dataset.cartPick === 'group' ? control.dataset.cartGroup : control.dataset.cartRule])) {
        toast('Added to cart');
        focusCheckout(control);
      }
    } else if (target.matches('[data-cart-add-group]')) {
      confirmed(control.dataset, () => {
        if (add([control.dataset.cartGroup])) {
          toast('Added to cart');
          focusCheckout(control);
        }
      });
    } else if (target.matches('[data-cart-remove]')) {
      remove(target.dataset.key);
      toast('Removed from cart');
      control.querySelector('[data-cart-open], [data-cart-add-group]')?.focus();
    } else if (target.matches('[data-cart-groups-add]')) {
      const panel = target.closest('[data-cart-groups]');
      const picked = [...document.querySelectorAll('[data-cart-pick-group]:checked')];
      confirmed(panel.dataset, () => {
        const added = add(picked.map((box) => box.dataset.cartPickGroup));
        picked.forEach((box) => (box.checked = false));
        paintGroups();
        if (added) toast(`Added ${count(added, 'group', 'groups')} to cart`);
      });
    } else if (target.matches('[data-cart-groups-all], [data-cart-groups-clear]')) {
      const on = target.matches('[data-cart-groups-all]');
      document.querySelectorAll('[data-cart-pick-group]').forEach((box) => (box.checked = on));
      paintGroups();
    }
  });

  document.addEventListener('change', (event) => {
    if (event.target.matches('[data-cart-pick-group]')) paintGroups();
  });

  // A click on the dialog's backdrop, outside its box, closes it, as the prototype's scrim does.
  dialog?.addEventListener('click', (event) => {
    if (event.target !== dialog) return;
    const box = dialog.getBoundingClientRect();
    const inside = event.clientX >= box.left && event.clientX <= box.right && event.clientY >= box.top && event.clientY <= box.bottom;
    if (!inside) dialog.close();
  });

  // ---------- The cart's page ----------

  const CHECKOUT = '/cart/checkout.json';
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

  /** Return a rule's row: its icon, title, group and version, Stay in sync or Fork, and Remove. */
  function ruleRow(item) {
    const note = leftOutNote(item);
    const title = item.href ? h('a', 'block leading-[1.35] font-semibold text-ink no-underline hover:underline', { href: item.href }, item.title)
      : h('span', 'block leading-[1.35] font-semibold', {}, item.title);
    const meta = note ? [note, item.state === 'retired' && item.href ? h('a', 'text-muted', { href: item.href }, 'See what replaced it') : null]
      : [`Rule in ${item.group.name}`, ' · ', item.version];
    let mode = h('span');
    if (item.state === 'ready') {
      const name = `mode-${item.key}`;
      const option = (value, label) => h('label', 'inline-flex cursor-pointer items-center rounded-full px-[11px] py-[3px] whitespace-nowrap text-muted has-checked:bg-surface-header has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-(--focus)', {},
        h('input', 'pointer-events-none absolute opacity-0', { type: 'radio', name, value, checked: (value === 'fork') === !!state.fork[item.key], 'data-cart-mode': item.key, 'data-focus': `mode:${item.key}:${value}` }),
        label);
      mode = h('div', 'relative inline-flex gap-0.5 rounded-full border border-border p-0.5 text-[12.5px]', { role: 'radiogroup', 'aria-label': `${item.title}: stay in sync or fork` },
        option('sync', 'Stay in sync'), option('fork', 'Fork'));
    }
    return h('li', 'grid grid-cols-[36px_minmax(0,1fr)_auto_28px] items-center gap-[14px] px-4 py-3 max-narrow:grid-cols-[36px_minmax(0,1fr)_28px] max-narrow:[&>:nth-child(3)]:col-start-2 max-narrow:[&>:nth-child(3)]:row-start-2 max-narrow:[&>:nth-child(3)]:justify-self-start max-narrow:[&>:nth-child(4)]:col-start-3 max-narrow:[&>:nth-child(4)]:row-start-1', {},
      h('span', 'grid size-9 place-items-center rounded-[9px] border border-border-subtle bg-paper text-muted', {}, docIcon()),
      h('div', 'min-w-0', {}, title, h('div', 'mt-[3px] flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-faint', {}, ...meta)),
      mode, removeButton(item));
  }

  /** Return a whole group's row, shaded: its tile, name and tag, how many rules and its ID, the rules it brings,
   * Stays in sync, and Remove. */
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
        rules.length ? h('ul', 'mt-2 list-disc pl-4 text-[12.5px] text-muted', {}, ...rules.map((r) => h('li', 'my-0.5', {}, r.title))) : null),
      h('span', 'mt-1.5 text-[13px] whitespace-nowrap text-faint', {}, item.state === 'ready' ? 'Stays in sync' : ''),
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
    if (lib.upsell) {
      parts.push(h('label', 'mt-2.5 flex cursor-pointer items-center gap-2 text-[13px] text-muted', {},
        h('input', 'size-4 accent-(--ink)', { type: 'checkbox', checked: lib.upsell.full, 'data-cart-full': lib.fullName, 'data-focus': `full:${lib.fullName}` }),
        `Also add the other ${listed(lib.upsell.groups)} rules${lib.upsell.full ? '' : ` (${lib.upsell.extra} more)`}`));
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
    // The lines the preview showed last, of its tab, so lines that change stand out briefly.
    let shown = { tab: null, lines: new Set() };
    repo.value = state.repo;

    /** Ask for the cart's checkout, a moment after the last change, so typing asks once. */
    const request = () => {
      clearTimeout(timer);
      timer = setTimeout(checkout, answer ? 200 : 0);
    };

    /** Ask Rulemart to resolve the cart and write its texts, keeping only the latest answer. */
    async function checkout() {
      if (!state.cart.length) return;
      const id = ++asked;
      try {
        const response = await fetch(CHECKOUT, {
          method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ cart: state.cart, fork: state.fork, full: state.full, confirmed: state.confirmed, repo: state.repo }),
        });
        if (!response.ok) throw new Error(`checkout answered ${response.status}`);
        const body = await response.json();
        if (id !== asked) return;
        answer = body;
        failed = false;
        if (body.unknown.length) {
          // Keys no page writes, such as from an older cart, name nothing to show.
          state.cart = state.cart.filter((key) => !body.unknown.includes(key));
          save();
        }
      } catch {
        if (id !== asked) return;
        failed = true;
      }
      show();
    }

    /** Show the cart: empty, or its cards from the latest answer, as much of it as the cart still holds. */
    function show() {
      const empty = state.cart.length === 0;
      $('[data-cart-empty]').hidden = !empty;
      $('[data-cart-full]').hidden = empty;
      if (empty) return;
      const libraries = (answer?.libraries || [])
        .map((lib) => ({ lib, items: lib.items.filter((item) => inCart(item.key)).map((item) => ({ ...item, fork: !!state.fork[item.key] })) }))
        .filter(({ items }) => items.length);
      const items = libraries.flatMap(({ items: held }) => held);
      const rules = items.filter((item) => item.kind === 'rule').length;
      const groups = items.length - rules;
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
      const invalid = !!answer?.repositoryInvalid && state.repo.trim() !== '';
      const named = answer?.repository && state.repo.trim() !== '';
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

    /** Say how rules move to newer versions, and on the Commands tab, how to pin a library to a release. */
    function showFootnote(prompt) {
      const code = (text) => h('code', '', {}, text);
      const parts = ['Rules move to newer versions only when your project runs ', code('code-rules project update'), '.'];
      const pinnable = answer?.libraries.find((lib) => lib.vetted && !lib.gone && lib.items.some((item) => item.state === 'ready' && inCart(item.key) && !state.fork[item.key]));
      if (!prompt && pinnable) {
        parts.push(' To pin a library to one release instead, add ', code(`--ref ${pinnable.release}`), ` to its add library command, ${pinnable.release} being ${pinnable.fullName}’s latest.`);
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
      const target = event.target.closest('[data-cart-drop], [data-cart-confirm-library], [data-cart-clear], [data-cart-tab], [data-cart-copy]');
      if (!target) return;
      if (target.matches('[data-cart-drop]')) {
        // Focus moves to the next item's Remove, or the one before, or Clear cart.
        const drops = [...root.querySelectorAll('[data-cart-drop]')];
        const at = drops.indexOf(target);
        const next = drops[at + 1] || drops[at - 1];
        remove(target.dataset.cartDrop);
        toast('Removed from cart');
        (next ? root.querySelector(`[data-focus="${CSS.escape(next.dataset.focus)}"]`) : $('[data-cart-clear]'))?.focus();
        if (!state.cart.length) root.querySelector('[data-cart-empty] a')?.focus();
      } else if (target.matches('[data-cart-confirm-library]')) {
        state.confirmed[target.dataset.cartConfirmLibrary] = true;
        save();
      } else if (target.matches('[data-cart-clear]')) {
        state.cart = [];
        state.fork = {};
        state.full = {};
        save();
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
        if (target.value === 'fork') state.fork[target.dataset.cartMode] = true;
        else delete state.fork[target.dataset.cartMode];
        save();
      } else if (target.matches('[data-cart-full]')) {
        if (target.checked) state.full[target.dataset.cartFull] = true;
        else delete state.full[target.dataset.cartFull];
        save();
      }
    });
    repo.addEventListener('input', () => {
      state.repo = repo.value.slice(0, 500);
      save();
    });
    // Each change to the cart, here or in another tab, shows at once, and asks for the texts again.
    window.addEventListener('rulemart:cart', () => {
      if (document.activeElement !== repo) repo.value = state.repo;
      show();
      request();
    });
    show();
  }

  const page = document.querySelector('[data-cart-page]');
  if (page) cartPage(page);

  // Another tab's change to the cart shows here too.
  window.addEventListener('storage', (event) => {
    if (event.key !== STORE && event.key !== null) return;
    state = load();
    paint();
  });

  paint();
})();
