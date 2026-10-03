/** @fileoverview The cart, which lives in the visitor's browser, as the prototype's does, so it needs no account and
 * the pages stay the same for everyone. It keeps localStorage's rulemart-cart: cart, the ordered keys of whole groups,
 * group::owner/repo::kind/group, and rules, owner/repo::kind/group/slug, at most 100; fork, the rules the visitor
 * forks; restOfGroups, the libraries whose picked rules' groups they add the rest of; repo, where checkout's texts go;
 * and confirmed, the unvetted libraries they confirmed adding from, both by owner/name in lowercase. Each choice lasts
 * only while the cart holds its rule or an item of its library, so the choices stay as small as the cart, which the
 * server bounds. It paints the header's count and each page's cart controls from the data attributes the page renders, opens the dialogs that add, and toasts what changed. The cart's
 * page, which cart-page.js renders, reads and changes the cart only through window.rulemartCart, and learns of each
 * change, here or in another tab, from the rulemart:cart event. Without JavaScript, or storage, there's no cart: the
 * stylesheet hides every control marked data-needs-script. */
(() => {
  const STORE = 'rulemart-cart';
  // How many items a cart holds, and how long a key may be, as the header's cart link says the server bounds them.
  const link = document.querySelector('[data-cart-link]');
  if (!link) return;
  const MAX_ITEMS = Number(link.dataset.cartMaxItems);
  const MAX_KEY = Number(link.dataset.cartMaxKeyLength);
  // A key as the server's ParseCartKey reads it: a library's owner and name, then a group's ID, two parts, or a
  // rule's, three or more, each part as Code Rules spells IDs.
  const NAME = '[A-Za-z0-9_.-]+';
  const PART = '[A-Za-z0-9][A-Za-z0-9._-]*';
  const RULE_KEY = new RegExp(`^${NAME}/${NAME}::${PART}(?:/${PART}){2,}$`);
  const GROUP_KEY = new RegExp(`^group::${NAME}/${NAME}::${PART}/${PART}$`);

  /** Report whether key is one the cart can hold. */
  const validKey = (key) => typeof key === 'string' && key.length <= MAX_KEY && (RULE_KEY.test(key) || GROUP_KEY.test(key));

  /** Report whether keys a and b name one item: the server finds libraries, groups, and rules without regard to case,
   * so keys that differ only in case name the same one. */
  const sameKey = (a, b) => a.toLowerCase() === b.toLowerCase();

  /** Return keys without those that name an item an earlier key names, in order. */
  const distinctKeys = (keys) => keys.filter((key, i) => keys.findIndex((other) => sameKey(other, key)) === i);

  /** Return the plural of word for n, with n: 1 rule, 2 rules. */
  const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

  const fresh = () => ({ cart: [], fork: {}, restOfGroups: {}, repo: '', confirmed: {} });

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

  /** Return library, as owner/name, as the cart keys its choices: in lowercase, since the server finds libraries without
   * regard to case, and the catalog's spelling of one changes when its repository's does. */
  const libraryKey = (library) => library.toLowerCase();

  /** Return the libraries of value, an object of flags, that are on, as an object of flags keyed by libraryKey, so an
   * older cart's spelling of a library is the one its page shows now. */
  const libraryFlags = (value) => Object.fromEntries(Object.keys(flags(value)).map((name) => [libraryKey(name), true]));

  /** Turn library's flag in libraries, an object of flags, on or off, in whichever case it's spelled there. */
  function setLibraryFlag(libraries, library, on) {
    const key = libraryKey(library);
    for (const name of Object.keys(libraries)) {
      if (libraryKey(name) === key) delete libraries[name];
    }
    if (on) libraries[key] = true;
  }

  /** Read the cart from localStorage, keeping only what a cart can hold: well-formed keys, each item once, at most
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
    if (Array.isArray(stored.cart)) state.cart = distinctKeys(stored.cart.filter(validKey)).slice(0, MAX_ITEMS);
    state.fork = Object.fromEntries(Object.keys(flags(stored.fork)).filter((key) => state.cart.includes(key)).map((key) => [key, true]));
    state.restOfGroups = libraryFlags(stored.restOfGroups);
    state.confirmed = libraryFlags(stored.confirmed);
    if (typeof stored.repo === 'string') state.repo = stored.repo.slice(0, 500);
    return state;
  }

  let state = load();

  /** Return the library, as owner/name, of the item key names. */
  const libraryOf = (key) => key.replace(/^group::/, '').split('::')[0];

  /** Drop the choices for what the cart no longer holds: forks of rules it doesn't hold, and choices for libraries it
   * holds no item of. */
  function prune() {
    const libraries = new Set(state.cart.map((key) => libraryKey(libraryOf(key))));
    const held = (choices, has) => Object.fromEntries(Object.keys(choices).filter(has).map((name) => [name, true]));
    state.fork = held(state.fork, (key) => state.cart.includes(key));
    state.restOfGroups = held(state.restOfGroups, (library) => libraries.has(libraryKey(library)));
    state.confirmed = held(state.confirmed, (library) => libraries.has(libraryKey(library)));
  }

  /** Keep the cart, without the choices for what it no longer holds, and paint every page part that shows it. A browser
   * that refuses storage keeps it for this page. */
  function save() {
    prune();
    try {
      localStorage.setItem(STORE, JSON.stringify(state));
    } catch {
      // Storage is full or refused: the cart lasts as long as this page.
    }
    paint();
  }

  /** Report whether the cart holds the item key names, in whichever case the cart spells it. */
  const inCart = (key) => !!key && state.cart.some((held) => sameKey(held, key));

  /** Return the key of the whole group of the rule key names, or '' for a group's key. */
  function groupKeyOf(key) {
    if (key.startsWith('group::')) return '';
    const [library, path] = key.split('::');
    return `group::${library}::${path.split('/').slice(0, 2).join('/')}`;
  }

  /** Return how many items the cart holds, a rule whose whole group it holds too counting with the group. */
  const itemCount = () => state.cart.filter((key) => !inCart(groupKeyOf(key))).length;
  const toast = (text) => window.rulemartToast?.(text);

  /** Add keys the cart doesn't hold, in order, while it has room, and return how many it added, saying so when the
   * cart is full. Keys it holds already count as added, since the visitor sees them in it. confirmed, when given, is
   * the library the visitor confirmed adding them from though Rulemart doesn't vet it, which is recorded with them,
   * since a confirmation lasts only while the cart holds an item of its library. */
  function add(keys, confirmed = '') {
    const adding = distinctKeys(keys.filter((key) => validKey(key) && !inCart(key)));
    const room = Math.max(MAX_ITEMS - state.cart.length, 0);
    state.cart.push(...adding.slice(0, room));
    if (confirmed) setLibraryFlag(state.confirmed, confirmed, true);
    save();
    if (adding.length > room) {
      toast(`Your cart holds ${MAX_ITEMS} items, as many as it can. Remove some, or add a whole group.`);
    }
    return keys.length - adding.length + Math.min(adding.length, room);
  }

  /** Remove the item key names from the cart, in whichever case the cart spells it, with its fork. */
  function remove(key) {
    for (const held of state.cart.filter((held) => sameKey(held, key))) delete state.fork[held];
    state.cart = state.cart.filter((held) => !sameKey(held, key));
    save();
  }

  /** Turn forking the rule key on or off. */
  function setFork(key, on) {
    if (on) state.fork[key] = true;
    else delete state.fork[key];
    save();
  }

  /** Turn adding the rest of the groups of library's picked rules on or off. */
  function setRestOfGroups(library, on) {
    setLibraryFlag(state.restOfGroups, library, on);
    save();
  }

  /** Record that the visitor confirmed adding from library, which Rulemart doesn't vet, and whose items the cart holds. */
  function confirm(library) {
    setLibraryFlag(state.confirmed, library, true);
    save();
  }

  /** Keep text as the project's repository, which checkout's texts name. */
  function setRepo(text) {
    state.repo = text.slice(0, 500);
    save();
  }

  /** Empty the cart, with every choice for its items, keeping the repository. */
  function clear() {
    state.cart = [];
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

  /** Paint the header's cart link: the count of items, a whole group counting once, with the rules of it the cart
   * holds too, in a badge, and in its name. */
  function paintCount() {
    const n = itemCount();
    for (const link of document.querySelectorAll('[data-cart-link]')) {
      link.setAttribute('aria-label', `Cart, ${count(n, 'item', 'items')}`);
      const badge = link.querySelector('[data-cart-count]');
      badge.textContent = String(n);
      badge.hidden = n === 0;
    }
  }

  /** Paint each control that adds a rule or a group: Add, or once the cart holds its item, or a rule's whole group,
   * In cart, Checkout, and Remove, which names the group when the cart holds the rule's group, which brings the rule
   * whether the cart holds the rule too or not. */
  function paintControls() {
    for (const control of document.querySelectorAll('[data-cart-control]')) {
      const { cartRule: rule, cartGroup: group, cartGroupName: groupName } = control.dataset;
      const held = inCart(group) ? group : inCart(rule) ? rule : '';
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

  /** Return the Groups tab's checkboxes that pick groups for panel, its Add to cart box: those of panel's library
   * whose group the cart doesn't hold yet. */
  const picksOf = (panel) => [...document.querySelectorAll(`[data-cart-pick-group][data-cart-library="${CSS.escape(panel.dataset.cartLibrary)}"]`)]
    .filter((box) => !box.disabled);

  /** Paint each Add to cart box of the library page from the groups ticked for it: how many, and its button's label.
   * A group the cart holds already shows ticked, and can't be picked again or counted. */
  function paintGroups() {
    for (const box of document.querySelectorAll('[data-cart-pick-group]')) {
      const held = inCart(box.dataset.cartPickGroup);
      if (box.disabled !== held) box.checked = box.disabled = held;
    }
    for (const panel of document.querySelectorAll('[data-cart-groups]')) {
      const n = picksOf(panel).filter((box) => box.checked).length;
      const updates = panel.dataset.cartVetted === 'false' ? 'are pinned to the commit you review' : `stay in sync with ${panel.dataset.cartLibrary}`;
      panel.querySelector('[data-cart-groups-text]').textContent = n
        ? `${count(n, 'group', 'groups')} selected. Whole groups ${updates}.`
        : 'Select whole groups to add. You can also add single rules from their pages.';
      panel.querySelector('[data-cart-groups-label]').textContent = n ? `Add ${count(n, 'group', 'groups')} to cart` : 'Add groups to cart';
      panel.querySelectorAll('[data-cart-groups-add]').forEach((button) => (button.disabled = n === 0));
      panel.querySelector('[data-cart-groups-clear]').hidden = n === 0;
      panel.querySelector('[data-cart-groups-bar]').hidden = n === 0;
      panel.querySelector('[data-cart-groups-count]').textContent = `${n} selected`;
    }
  }

  const dialog = document.querySelector('dialog[data-cart-dialog]');
  // What the open dialog acts on: opener, the control or Add to cart box that opened it, whose item its choices add,
  // opener's library, which confirming passes to then, what confirming goes on to do: show the dialog's choices, or
  // add what the visitor asked to, with the library confirmed; and confirmed, that library once the visitor confirmed
  // it and the dialog shows its choices. Null while the dialog is closed.
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

  /** Run then with the library the visitor confirmed adding from, after they confirm adding from opener's library
   * when opener, the control that asks, says it isn't vetted; at once, with none, otherwise. */
  function confirmed(opener, then) {
    if (opener.dataset.cartVetted !== 'false' || !dialog) {
      then('');
      return;
    }
    openDialog(opener, 'confirm', (library) => {
      dialog.close();
      then(library);
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
      // A rule page's dialog: for an unvetted library, the warning first, then the choices, the first of them focused,
      // since the button that confirmed is gone.
      openDialog(control, control.dataset.cartVetted === 'false' ? 'confirm' : 'choose', (library) => {
        pending.confirmed = library;
        showStep('choose');
        dialog.querySelector('[data-cart-pick]').focus();
      });
    } else if (target.matches('[data-cart-confirm]')) {
      // The confirmation is recorded with what it adds, if the visitor goes on to add anything.
      opened.then(opened.library);
    } else if (target.matches('[data-cart-close]')) {
      dialog.close();
    } else if (target.matches('[data-cart-pick]')) {
      dialog.close();
      const { opener } = opened;
      if (add([target.dataset.cartPick === 'group' ? opener.dataset.cartGroup : opener.dataset.cartRule], opened.confirmed)) {
        toast('Added to cart');
        focusCheckout(opener);
      }
    } else if (target.matches('[data-cart-add-group]')) {
      confirmed(control, (library) => {
        if (add([control.dataset.cartGroup], library)) {
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
      confirmed(panel, (library) => {
        const added = add(picked.map((box) => box.dataset.cartPickGroup), library);
        // The cart holds what it added, whose boxes now show so; the rest, if it filled up, go back unticked.
        picked.filter((box) => !box.disabled).forEach((box) => (box.checked = false));
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
    /** Return a copy of the cart: cart, fork, restOfGroups, repo, and confirmed, as localStorage keeps them. */
    state: () => structuredClone(state),
    inCart,
    add,
    remove,
    setFork,
    setRestOfGroups,
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
