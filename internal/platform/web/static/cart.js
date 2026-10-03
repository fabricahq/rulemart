/** @fileoverview The cart, which lives in the visitor's browser, as the prototype's does, so it needs no account and
 * the pages stay the same for everyone. It keeps localStorage's rulemart-cart: cart, the ordered keys of whole groups,
 * group::owner/repo::kind/group, and rules, owner/repo::kind/group/slug, at most 100; fork, the rules the visitor
 * forks; full, the libraries whose picked rules' groups they add whole; project and repo, where checkout's texts go;
 * and confirmed, the unvetted libraries they confirmed adding from. It paints the header's count and each page's cart
 * controls from the data attributes the page renders, opens the dialogs that add, and toasts what changed. The cart's
 * page, which cart-page.js renders, reads and changes the cart only through window.rulemartCart, and learns of each
 * change, here or in another tab, from the rulemart:cart event. Without JavaScript, or storage, there's no cart: the
 * stylesheet hides every control marked data-needs-script. */
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

  /** Turn forking the rule key on or off. */
  function setFork(key, on) {
    if (on) state.fork[key] = true;
    else delete state.fork[key];
    save();
  }

  /** Turn adding the rest of the groups of library's picked rules on or off. */
  function setFull(library, on) {
    if (on) state.full[library] = true;
    else delete state.full[library];
    save();
  }

  /** Record that the visitor confirmed adding from library, which Rulemart doesn't vet. */
  function confirm(library) {
    state.confirmed[library] = true;
    save();
  }

  /** Keep text as the project's repository, which checkout's texts name. */
  function setRepo(text) {
    state.repo = text.slice(0, 500);
    save();
  }

  /** Empty the cart, with its forks and choices of whole groups, keeping the repository and the libraries confirmed. */
  function clear() {
    state.cart = [];
    state.fork = {};
    state.full = {};
    save();
  }

  /** Drop keys, which checkout says name nothing a cart can hold, such as an older cart's, from the cart. */
  function dropUnknown(keys) {
    state.cart = state.cart.filter((key) => !keys.includes(key));
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

  /** Return the Groups tab's checkboxes that pick groups for panel, its Add to cart box: those of panel's library. */
  const picksOf = (panel) => [...document.querySelectorAll(`[data-cart-pick-group][data-cart-library="${CSS.escape(panel.dataset.cartLibrary)}"]`)];

  /** Paint each Add to cart box of the library page from the groups ticked for it: how many, and its button's label. */
  function paintGroups() {
    for (const panel of document.querySelectorAll('[data-cart-groups]')) {
      const n = picksOf(panel).filter((box) => box.checked).length;
      panel.querySelector('[data-cart-groups-text]').textContent = n
        ? `${count(n, 'group', 'groups')} selected. Whole groups stay in sync with ${panel.dataset.cartLibrary}.`
        : 'Select whole groups to add. You can also add single rules from their pages.';
      panel.querySelector('[data-cart-groups-label]').textContent = n ? `Add ${count(n, 'group', 'groups')} to cart` : 'Add groups to cart';
      panel.querySelector('[data-cart-groups-add]').disabled = n === 0;
      panel.querySelector('[data-cart-groups-clear]').hidden = n === 0;
    }
  }

  const dialog = document.querySelector('dialog[data-cart-dialog]');
  // What the open dialog acts on: opener, the control or Add to cart box that opened it, whose item its choices add,
  // opener's library, which confirming records, and then, what confirming goes on to do: show the dialog's choices, or
  // add what the visitor asked to. Null while the dialog is closed.
  let pending = null;

  /** Show the dialog's step, confirm or choose, and hide the other. */
  function showStep(step) {
    for (const part of dialog.querySelectorAll('[data-cart-step]')) part.hidden = part.dataset.cartStep !== step;
  }

  /** Open the dialog at step for opener, which confirming, then, goes on from. */
  function openDialog(opener, step, then) {
    pending = { opener, library: opener.dataset.cartLibrary, then };
    showStep(step);
    dialog.showModal();
  }

  /** Run then, after the visitor confirms adding from opener's library when opener, the control that asks, says it
   * isn't vetted; at once otherwise. */
  function confirmed(opener, then) {
    if (opener.dataset.cartVetted !== 'false' || !dialog) {
      then();
      return;
    }
    openDialog(opener, 'confirm', () => {
      dialog.close();
      then();
    });
  }

  /** Focus the Checkout link of control, once it shows the cart holds its item. */
  const focusCheckout = (control) => control.querySelector('[data-cart-held] a')?.focus();

  document.addEventListener('click', (event) => {
    const target = event.target.closest(
      '[data-cart-open], [data-cart-pick], [data-cart-confirm], [data-cart-close], [data-cart-remove], [data-cart-add-group], [data-cart-groups-add], [data-cart-groups-all], [data-cart-groups-clear]',
    );
    if (!target) return;
    const control = target.closest('[data-cart-control]');
    const opened = pending;
    if (target.matches('[data-cart-open]')) {
      // A rule page's dialog: for an unvetted library, the warning first, then the choices.
      openDialog(control, control.dataset.cartVetted === 'false' ? 'confirm' : 'choose', () => showStep('choose'));
    } else if (target.matches('[data-cart-confirm]')) {
      confirm(opened.library);
      opened.then();
    } else if (target.matches('[data-cart-close]')) {
      dialog.close();
    } else if (target.matches('[data-cart-pick]')) {
      dialog.close();
      const { opener } = opened;
      if (add([target.dataset.cartPick === 'group' ? opener.dataset.cartGroup : opener.dataset.cartRule])) {
        toast('Added to cart');
        focusCheckout(opener);
      }
    } else if (target.matches('[data-cart-add-group]')) {
      confirmed(control, () => {
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
      const picked = picksOf(panel).filter((box) => box.checked);
      confirmed(panel, () => {
        const added = add(picked.map((box) => box.dataset.cartPickGroup));
        picked.forEach((box) => (box.checked = false));
        paintGroups();
        if (added) toast(`Added ${count(added, 'group', 'groups')} to cart`);
      });
    } else if (target.matches('[data-cart-groups-all], [data-cart-groups-clear]')) {
      const on = target.matches('[data-cart-groups-all]');
      picksOf(target.closest('[data-cart-groups]')).forEach((box) => (box.checked = on));
      paintGroups();
    }
  });

  document.addEventListener('change', (event) => {
    if (event.target.matches('[data-cart-pick-group]')) paintGroups();
  });

  dialog?.addEventListener('close', () => (pending = null));

  // A click on the dialog's backdrop, outside its box, closes it, as the prototype's scrim does.
  dialog?.addEventListener('click', (event) => {
    if (event.target !== dialog) return;
    const box = dialog.getBoundingClientRect();
    const inside = event.clientX >= box.left && event.clientX <= box.right && event.clientY >= box.top && event.clientY <= box.bottom;
    if (!inside) dialog.close();
  });

  // The cart's store, which the cart's page, cart-page.js, changes the cart through, and the rulemart:cart event, which
  // every change sends once the page's controls show it.
  window.rulemartCart = {
    /** Return a copy of the cart: cart, fork, full, repo, and confirmed, as localStorage keeps them. */
    state: () => structuredClone(state),
    inCart,
    add,
    remove,
    setFork,
    setFull,
    confirm,
    setRepo,
    clear,
    dropUnknown,
  };

  // Another tab's change to the cart shows here too.
  window.addEventListener('storage', (event) => {
    if (event.key !== STORE && event.key !== null) return;
    state = load();
    paint();
  });

  paint();
})();
