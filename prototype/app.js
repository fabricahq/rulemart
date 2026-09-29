/** @fileoverview Rulemart click-through mock: a hash-routed single page app that renders every Rulemart screen and simulates GitHub sign-in, app install, and issue creation. State lives in localStorage so flows chain together. */

(() => {
  const D = window.RULEMART_DATA;
  const STORE = 'rulemart-mock-v1';

  // ---------- State ----------
  const fresh = () => ({ signedIn: false, stars: {}, added: [], private: false, issues: [], theme: 'system', returnTo: null });
  let state = fresh();
  try { state = { ...fresh(), ...JSON.parse(localStorage.getItem(STORE) || '{}') }; } catch { /* storage unavailable: run in memory */ }
  const save = () => { try { localStorage.setItem(STORE, JSON.stringify(state)); } catch { /* ignore */ } };

  // ---------- Helpers ----------
  const esc = s => String(s ?? '').replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
  const fmt = n => n.toLocaleString('en-US');
  const usedBy = n => (n ? `Used by ${fmt(n)}` : 'Not used yet');
  const $ = sel => document.querySelector(sel);
  const user = D.user;

  const icon = {
    search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>',
    gh: '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>',
    fab: '<svg viewBox="0 0 24 24" fill="none"><g stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M12 1.75 21.25 7.1V17L12 22.35 2.75 17V7.1Z"/><path d="m2.75 7.1 9.25 5.35 9.25-5.35M12 12.45v9.9"/><path d="m7.375 4.425 9.25 5.35M7.375 9.775v9.9M12 17.4l9.25-5.35"/></g></svg>',
    star: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9Z"/></svg>',
    starOn: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9Z"/></svg>',
    chat: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="M4 5h16v11H9l-5 4Z"/></svg>',
  };
  // Small library mark for quiet rows: Fabrica's logo on a dark tile, otherwise the owner's avatar.
  const libMark = lib => (isFabrica(lib) ? `<span class="fabmark" title="Published by Fabrica">${icon.fab}</span>` : avatar(lib.owner, 'xs'));
  const fabBadge = () => `<span class="fab" title="Published by Fabrica">${icon.fab}Fabrica</span>`;
  // GitHub-style identicon: a mirrored 5x5 grid whose color and pattern come from a hash of the login.
  function identicon(login) {
    let h = 2166136261;
    for (const ch of login) { h ^= ch.charCodeAt(0); h = Math.imul(h, 16777619) >>> 0; }
    const color = `hsl(${h % 360} 55% 55%)`;
    let cells = '';
    for (let y = 0; y < 5; y += 1) for (let x = 0; x < 3; x += 1) {
      if ((h >>> ((y * 3 + x) % 31)) & 1) { cells += `<rect x="${x + 1}" y="${y + 1}" width="1" height="1"/>`; if (x < 2) cells += `<rect x="${5 - x}" y="${y + 1}" width="1" height="1"/>`; }
    }
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 7 7" shape-rendering="crispEdges"><rect width="7" height="7" fill="#f0f0f0"/><g fill="${color}">${cells}</g></svg>`;
    return `data:image/svg+xml,${encodeURIComponent(svg)}`;
  }
  const avatarSrc = login => ((D.owners[login] || {}).real ? `https://github.com/${login}.png?size=96` : identicon(login));
  const avatar = (owner, cls = '') => {
    const o = D.owners[owner] || { initials: owner.slice(0, 2).toUpperCase(), type: 'user' };
    const av = `<span class="avatar ${cls} ${o.type === 'org' ? 'org' : ''}"><img src="${avatarSrc(owner)}" alt="" onload="this.parentNode.classList.add('has-img')" onerror="this.remove()">${esc(o.initials)}</span>`;
    // Fabrica's avatar carries a small blue check instead of a separate badge.
    return owner === 'fabricahq' ? `<span class="av-wrap ${cls}" title="Published by Fabrica">${av}<span class="vcheck" aria-label="Published by Fabrica"><svg viewBox="0 0 12 12"><path d="M3.2 6.2 5.1 8l3.7-4" fill="none" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg></span></span>` : av;
  };

  // Libraries on Rulemart: fixtures plus anything the user published during the demo.
  const libraries = () => {
    const extra = state.added.map(id => D.publishable.find(p => p.id === id)).filter(Boolean)
      .map(p => ({ ...p.library, addedBy: user.login, addedOn: 'today', fresh: true }));
    return [...D.libraries, ...extra];
  };
  const libById = id => libraries().find(l => l.id === id);
  const libName = lib => lib.name || lib.id;
  const isFabrica = lib => lib.owner === 'fabricahq';
  const latest = lib => lib.tags[lib.tags.length - 1].v;
  const allRules = () => libraries().flatMap(lib => lib.rules.map(r => ({ ...r, lib, key: `${lib.id}::${r.group}/${r.slug}` })));
  const ruleUrl = r => `#/r/${r.lib.id}/${r.group}/${r.slug}`;
  const libUrl = (lib, tab) => `#/l/${lib.id}${tab ? `?tab=${tab}` : ''}`;
  const starCount = r => r.stars + (state.stars[r.key] ? 1 : 0);
  const isMaintainer = lib => state.signedIn && (lib.owner === user.login || user.orgs.includes(lib.owner));
  const groupName = id => (D.groups[id] || { name: id }).name;
  const isCanonical = id => (D.groups[id] || {}).canonical === true;
  // Code language for a rule's examples: an explicit override, else its group's language (practices default to TypeScript).
  const groupLang = { 'techs/go': 'go', 'techs/golang': 'go', 'techs/typescript': 'typescript', 'techs/react': 'typescript', 'techs/playwright': 'typescript', 'techs/nextjs': 'typescript', 'techs/python': 'python', 'techs/rust': 'rust', 'techs/postgresql': 'sql', 'techs/docker': 'dockerfile', 'techs/kubernetes': 'yaml', 'techs/terraform': 'ini' };
  const ruleLang = r => r.lang || groupLang[r.group] || 'typescript';
  // Technology names are self-explanatory; only practices show their reading guidance.
  const groupBlurb = id => (id.startsWith('practices/') ? (D.groups[id] || {}).whenToRead || '' : '');
  // Canonical groups carry an icon: Devicon logos (MIT) for technologies, Lucide line icons (ISC) for practices. Non-canonical groups have none.
  const iconUrl = g => (g.iconUrl || (g.icon ? `https://cdn.jsdelivr.net/gh/devicons/devicon@v2.17.0/icons/${g.icon}.svg` : g.lucide ? `https://cdn.jsdelivr.net/npm/lucide-static@1.48.0/icons/${g.lucide}.svg` : null));
  const techIcon = (id, cls = '') => { const g = D.groups[id]; const url = g && g.canonical && iconUrl(g); return url ? `<span class="ticon ${cls} ${g.lucide ? 'line' : ''} ${g.iconUrl ? 'wide' : ''}"><img src="${url}" alt=""></span>` : ''; };
  const alias = lib => lib.owner.replace(/hq$/, '').replace(/[^a-z0-9-]/gi, '');
  // Each rule has its own semver history. Release events (lib.tags) only supply dates and ordering.
  const releaseOf = (lib, v) => { const i = lib.tags.findIndex(t => t.v === v); return { i, date: i >= 0 ? lib.tags[i].date : '' }; };
  function ruleVersions(r) {
    const first = releaseOf(r.lib, r.added);
    let [maj, min, pat] = [1, 0, 0];
    const out = [{ version: '1.0.0', bump: 'new', summary: 'First published.', date: first.date, i: first.i }];
    [...r.changes].sort((a, b) => releaseOf(r.lib, a.v).i - releaseOf(r.lib, b.v).i).forEach(c => {
      if (c.bump === 'major') { maj += 1; min = 0; pat = 0; } else if (c.bump === 'minor') { min += 1; pat = 0; } else { pat += 1; }
      const rel = releaseOf(r.lib, c.v);
      out.push({ version: `${maj}.${min}.${pat}`, bump: c.bump, summary: c.summary, diff: c.diff, date: rel.date, i: rel.i });
    });
    return out.reverse();
  }
  const ruleVersion = r => ruleVersions(r)[0].version;
  // Version rows only call out the latest version; the version numbers say the rest.
  const versionChips = (v, isLatest) => (isLatest ? '<span class="chip">Latest</span>' : '');
  const issuesFor = ruleKey => state.issues.filter(i => i.ruleKey === ruleKey);
  const discussionFor = r => [...issuesFor(r.key).map(i => ({ kind: 'issue', num: i.num, title: i.title, author: i.author, comments: 0, state: 'open', when: 'just now', mine: true })), ...(r.discussion || [])];

  function nextIssueNum(repo) {
    const lib = libById(repo);
    const nums = [
      ...(lib ? lib.rules.flatMap(r => (r.discussion || []).map(d => d.num)) : []),
      ...state.issues.filter(i => i.repo === repo).map(i => i.num),
      repo === 'fabricahq/code-rules' ? 67 : 0,
    ];
    return Math.max(10, ...nums) + 1;
  }

  // ---------- Router ----------
  function parse() {
    const raw = location.hash.replace(/^#\/?/, '');
    const [path, qs] = raw.split('?');
    return { parts: path ? path.split('/').map(decodeURIComponent) : [], q: new URLSearchParams(qs || '') };
  }
  const go = href => { location.hash = href.replace(/^#/, ''); };
  function setQuery(update) {
    const { parts, q } = parse();
    Object.entries(update).forEach(([k, v]) => (v === null || v === '' || v === false ? q.delete(k) : q.set(k, v)));
    const qs = q.toString();
    history.replaceState(null, '', `#/${parts.map(encodeURIComponent).join('/').replace(/%2F/g, '/')}${qs ? `?${qs}` : ''}`);
    render(false);
  }

  // ---------- Chrome ----------
  function header(active) {
    const signed = state.signedIn
      ? `<div class="menu"><button class="avatar" data-act="menu" aria-label="Account menu" style="cursor:pointer;padding:0"><img src="${avatarSrc(user.login)}" alt="" onload="this.parentNode.classList.add('has-img')" onerror="this.remove()">${user.initials}</button>
          <div class="menu-pop hidden" id="menu"><div class="who"><b>${user.name}</b><div class="faint">@${user.login}</div></div>
          <a href="#/me">Dashboard</a><a href="#/me/add">Add a library</a><a href="#/me?tab=stars">Starred rules</a><button data-act="signout">Sign out</button></div></div>`
      : `<a class="btn small" href="#/signin" data-act="remember">${icon.gh}Sign in with GitHub</a>`;
    return `<header class="top"><div class="wrap">
      <div class="brand"><a class="fab-link" href="https://fabricahq.com">${icon.fab}Fabrica</a><span class="slash">/</span><a href="#/" aria-label="Rulemart home">Rulemart</a></div>
      <label class="topsearch">${icon.search}<input id="topq" placeholder="Search rules" value="${esc(active === 'search' ? parse().q.get('q') || '' : '')}" aria-label="Search rules"></label>
      <nav class="nav">
        <a href="#/browse/techs" class="hide-md ${active === 'techs' ? 'cur' : ''}">Technologies</a>
        <a href="#/browse/practices" class="hide-md ${active === 'practices' ? 'cur' : ''}">Practices</a>
        <a href="#/libraries" class="hide-md ${active === 'libraries' ? 'cur' : ''}">Libraries</a>
        <a href="#" data-act="feedback" class="hide-md">Feedback</a>
        ${signed}
      </nav></div></header>`;
  }
  function footer() {
    const themeLabel = { system: 'Theme: system', light: 'Theme: light', dark: 'Theme: dark' }[state.theme];
    return `<footer class="foot"><div class="wrap">
      <div class="row"><span>Fabrica / Rulemart</span><span class="mocktag">Click-through mock · all data invented</span></div>
      <div class="row" style="gap:18px"><a href="#" data-act="feedback">Feedback on Code Rules</a><button data-act="theme">${themeLabel}</button><button data-act="reset">Reset demo</button></div>
    </div></footer>`;
  }

  // ---------- Shared bits ----------
  const impact = i => `<span class="impact ${i}">${i}</span>`;
  function ruleResult(r, q, { showGroup = true } = {}) {
    const hl = s => (q ? esc(s).replace(new RegExp(`(${q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'ig'), '<mark>$1</mark>') : esc(s));
    return `<a class="result" href="${ruleUrl(r)}">
      <div class="t"><span>${hl(r.title)}</span> ${impact(r.impact)}</div>
      <div class="meta"><span class="row" style="gap:7px">${libMark(r.lib)}${esc(libName(r.lib))}${showGroup ? ` · ${groupName(r.group)}` : ''}</span><span>${usedBy(r.usedBy)}</span><span>★ ${fmt(starCount(r))}</span></div></a>`;
  }
  function originCards(lib) {
    const o = D.owners[lib.owner] || { type: 'user', verified: null };
    return `<div class="origin">
      <div><span class="ol">Owner</span><a href="#/o/${lib.owner}"><b>${esc(lib.owner)}</b></a> · ${o.type === 'org' ? 'GitHub organization' : 'Personal account'} ${o.verified ? `<span class="verified">✓ ${o.verified}</span>` : ''}</div>
      <div><span class="ol">Repository</span><a href="#" data-act="noop">github.com/${esc(lib.id)}</a></div>
      <div><span class="ol">Added to Rulemart</span>by <b>@${esc(lib.addedBy)}</b> · ${esc(lib.addedOn)}</div></div>`;
  }
  function libLine(lib) {
    return `<a class="rowlink" href="${libUrl(lib)}"><div class="row" style="flex-wrap:nowrap;align-items:flex-start;gap:14px">${avatar(lib.owner, 'md')}<div>
      <div class="t">${esc(libName(lib))}</div>
      <div class="sm muted" style="margin-top:2px">${esc(lib.description)}</div>
      <div class="s" style="margin-top:4px"><span class="mono">${esc(lib.id)}</span> · ${lib.rules.length} rules · ${usedBy(lib.usedBy).replace('Used by', 'used by')}</div></div></div><span></span><span class="chev" aria-hidden="true">›</span></a>`;
  }
  const rowList = rows => `<div class="rowlist">${rows}</div>`;

  // ---------- Pages ----------
  function home() {
    const rules = allRules();
    const count = kind => Object.keys(D.groups).filter(g => g.startsWith(kind) && isCanonical(g)).map(g => ({ g, n: rules.filter(r => r.group === g).length, libs: new Set(rules.filter(r => r.group === g).map(r => r.lib.id)).size })).filter(x => x.n);
    const tiles = kind => count(kind).sort((a, b) => b.n - a.n).map(x => `<a class="tile tile-tech" href="#/g/${x.g}">${techIcon(x.g)}<div><b>${groupName(x.g)}</b><div class="sub">${x.n} ${x.n === 1 ? 'rule' : 'rules'} · ${x.libs} ${x.libs === 1 ? 'library' : 'libraries'}</div></div></a>`).join('');
    return `<section class="hero wrap">
      <p class="index">Fabrica / Rulemart</p>
      <h1>Agent coding best practices,<br class="br-wide"> off the shelf</h1>
      <p class="lede">No need to reinvent the wheel. Pick proven rules from other teams and add them straight to your codebase.</p>
      <form class="bigsearch" data-form="search">${icon.search}<input name="q" placeholder="Try React effects, logging, testing" aria-label="Search rules" autocomplete="off"></form>
      <div class="try">Popular <a class="chip" href="#/g/techs/typescript">TypeScript</a><a class="chip" href="#/g/techs/react">React</a><a class="chip" href="#/g/practices/testing">Testing</a><a class="chip" href="#/g/practices/error-handling">Error handling</a></div>
    </section>
    <section class="band"><div class="wrap">
      <div class="band-h"><h2>Technologies</h2><a href="#/browse/techs">Browse all →</a></div><div class="grid4">${tiles('techs/')}</div>
      <div class="band-h" style="margin-top:34px"><h2>Practices</h2><a href="#/browse/practices">Browse all →</a></div><div class="grid4">${tiles('practices/')}</div>
    </div></section>
    <section class="band"><div class="wrap">
      <div class="band-h"><h2>Libraries</h2><a href="#/libraries">All libraries →</a></div>
      ${rowList(libraries().slice(0, 4).map(libLine).join(''))}
    </div></section>
    <section class="band"><div class="wrap"><div class="cta-band">
      <div><p class="index" style="margin-bottom:6px">Stock the shelves</p><h3>Have a public Code Rules library?</h3><p class="muted" style="margin:6px 0 0">List it on Rulemart in a minute. Rulemart updates on every new tag.</p></div>
      <a class="btn primary" href="${state.signedIn ? '#/me/add' : '#/signin'}" data-act="${state.signedIn ? '' : 'remember-add'}">List your library →</a>
    </div></div></section>`;
  }

  // ---------- Rule filters (shared by search and group pages) ----------
  function readFilters(q) {
    return {
      kind: q.get('kind') || '', imp: q.get('impact') || '', onlyFab: q.get('fabrica') === '1', mine: q.get('mine') === '1',
      libs: new Set((q.get('libs') || '').split(',').filter(Boolean)), stars: Number(q.get('stars') || 0), used: Number(q.get('used') || 0),
    };
  }
  function applyFilters(items, f) {
    const myLibs = new Set(myLibraryIds());
    return items.filter(({ r }) => (!f.kind || r.group.startsWith(f.kind))
      && (f.imp !== 'high' || ['CRITICAL', 'HIGH'].includes(r.impact))
      && (f.imp !== 'medium' || !['CRITICAL', 'HIGH'].includes(r.impact))
      && (!f.onlyFab || isFabrica(r.lib)) && (!f.mine || myLibs.has(r.lib.id))
      && (!f.libs.size || f.libs.has(r.lib.id)) && starCount(r) >= f.stars && r.usedBy >= f.used);
  }
  function filterSidebar(base, f, { kind = false } = {}) {
    const cb = (key, val, label, on) => `<label><input type="checkbox" data-filter="${key}" data-val="${val}" ${on ? 'checked' : ''}> ${label}</label>`;
    const radios = (key, cur, opts) => opts.map(([v, l]) => `<label><input type="radio" name="${key}" data-radio="${key}" value="${v}" ${String(cur) === String(v) ? 'checked' : ''}> ${l}</label>`).join('');
    const libs = [...new Set(base.map(x => x.r.lib.id))].map(libById).sort((a, b) => (isFabrica(b) ? 1 : 0) - (isFabrica(a) ? 1 : 0) || b.usedBy - a.usedBy);
    const active = f.kind || f.imp || f.onlyFab || f.mine || f.libs.size || f.stars || f.used;
    return `<aside class="filters">
      <div class="fg"><h5>Libraries</h5>${state.signedIn ? cb('mine', '1', 'My libraries', f.mine) : ''}${libs.map(l => `<label class="${isFabrica(l) ? 'fab-row' : ''}"><input type="checkbox" data-multi="libs" data-val="${esc(l.id)}" ${f.libs.has(l.id) ? 'checked' : ''}> ${avatar(l.owner, 'xs')}<span class="trunc" title="${esc(libName(l))}">${esc(libName(l))}</span><span class="faint">${base.filter(x => x.r.lib.id === l.id).length}</span></label>`).join('')}</div>
      ${kind ? `<div class="fg"><h5>Kind</h5>${cb('kind', 'techs/', 'Technologies', f.kind === 'techs/')}${cb('kind', 'practices/', 'Practices', f.kind === 'practices/')}</div>` : ''}
      <div class="fg"><h5>Impact</h5>${cb('impact', 'high', 'Critical and high', f.imp === 'high')}${cb('impact', 'medium', 'Medium and lower', f.imp === 'medium')}</div>
      <div class="fg"><h5>Stars</h5>${radios('stars', f.stars, [[0, 'Any'], [10, '10+'], [50, '50+'], [100, '100+']])}</div>
      <div class="fg"><h5>Used by</h5>${radios('used', f.used, [[0, 'Any'], [100, '100+ projects'], [500, '500+ projects'], [1000, '1,000+ projects']])}</div>
      ${active ? '<button class="chip" data-clearfilters>Clear filters</button>' : ''}
    </aside>`;
  }
  const sorters = {
    best: (a, b) => b.score - a.score || b.r.usedBy - a.r.usedBy, used: (a, b) => b.r.usedBy - a.r.usedBy,
    stars: (a, b) => starCount(b.r) - starCount(a.r), new: (a, b) => (b.r.lib.fresh ? 1 : 0) - (a.r.lib.fresh ? 1 : 0) || b.r.net30 - a.r.net30,
  };
  function resultsList(rows, sort, sortOptions, term, emptyHtml, { grouped = false } = {}) {
    const libsN = new Set(rows.map(x => x.r.lib.id)).size;
    let body;
    if (!rows.length) body = emptyHtml;
    else if (grouped) {
      // Groups appear in the order of their best-ranked rule; rules keep their sort order inside each group.
      const order = []; const byGroup = {};
      rows.forEach(x => { if (!byGroup[x.r.group]) { byGroup[x.r.group] = []; order.push(x.r.group); } byGroup[x.r.group].push(x); });
      body = order.map(g => `<a class="grp-h" href="#/g/${g}">${techIcon(g, 'xs')}<span>${groupName(g)}</span><span class="mono faint">${g}</span><span class="n">${byGroup[g].length}</span></a>${byGroup[g].map(x => ruleResult(x.r, term, { showGroup: false })).join('')}`).join('');
    } else body = rows.map(x => ruleResult(x.r, term, { showGroup: false })).join('');
    return `<div>
      <div class="row between" style="margin-bottom:12px"><span class="muted sm">${rows.length} ${rows.length === 1 ? 'rule' : 'rules'} in ${libsN} ${libsN === 1 ? 'library' : 'libraries'}</span>
        <div class="seg">${sortOptions.map(([k, l]) => `<button data-sort="${k}" data-default="${sortOptions[0][0]}" class="${sort === k ? 'on' : ''}">${l}</button>`).join('')}</div></div>
      ${body}</div>`;
  }

  function search() {
    const { q } = parse();
    const term = (q.get('q') || '').trim();
    const f = readFilters(q);
    const sort = q.get('sort') || 'best';
    const t = term.toLowerCase();
    const base = allRules().map(r => {
      const text = r.body.replace(/<[^>]+>/g, ' ').toLowerCase();
      let score = 0;
      if (!t) score = 1;
      else {
        if (r.title.toLowerCase().includes(t)) score += 3;
        if (r.whenToRead.toLowerCase().includes(t)) score += 2;
        if (r.tags.some(x => x.includes(t)) || groupName(r.group).toLowerCase().includes(t)) score += 2;
        if (text.includes(t)) score += 1;
        if (t.endsWith('s') && score === 0 && (r.title + r.whenToRead + text).toLowerCase().includes(t.slice(0, -1))) score = 1;
      }
      return { r, score };
    }).filter(x => x.score > 0);
    const rows = applyFilters(base, f).sort(sorters[sort]);
    return `<div class="page wrap">
      <p class="index">Search</p><h1 class="title-xl" style="margin:8px 0 22px">${term ? `Rules matching “${esc(term)}”` : 'All rules'}</h1>
      <div class="search-layout">${filterSidebar(base, f, { kind: true })}
        ${resultsList(rows, sort, [['best', 'Best match'], ['used', 'Most used'], ['stars', 'Most starred'], ['new', 'Newest']], term, `<div class="empty">No rules match. Try a broader word, or <a href="#" data-act="feedback">tell us what you were looking for</a>.</div>`, { grouped: true })}
      </div></div>`;
  }

  function browse(kind, showOthers) {
    const prefix = `${kind}/`;
    const title = kind === 'techs' ? 'Technologies' : 'Practices';
    const rules = allRules();
    const rows = Object.keys(D.groups).filter(g => g.startsWith(prefix) && isCanonical(g)).map(g => {
      const rs = rules.filter(r => r.group === g); if (!rs.length) return '';
      const libs = new Set(rs.map(r => r.lib.id)).size;
      return `<a class="rowlink" href="#/g/${g}"><div class="row" style="flex-wrap:nowrap;gap:14px">${techIcon(g, 'md')}<div><div class="t">${groupName(g)}</div><div class="s">${g}${groupBlurb(g) ? ` · ${esc(groupBlurb(g))}` : ''}</div></div></div><div class="meta"><span>${rs.length} ${rs.length === 1 ? 'rule' : 'rules'}</span><span>${libs} ${libs === 1 ? 'library' : 'libraries'}</span></div><span class="chev" aria-hidden="true">›</span></a>`;
    }).join('');
    const others = Object.keys(D.groups).filter(g => g.startsWith(prefix) && !isCanonical(g)).map(g => ({ g, rs: rules.filter(r => r.group === g) })).filter(x => x.rs.length);
    const otherRows = others.map(({ g, rs }) => {
      const sim = D.groups[g].similarTo;
      return `<a class="rowlink" href="#/g/${g}"><div><div class="t mono" style="font-weight:500">${g}</div><div class="s">${esc(libName(rs[0].lib))}${sim ? ` · similar to ${groupName(sim)}` : ''}</div></div><div class="meta"><span>${rs.length} ${rs.length === 1 ? 'rule' : 'rules'}</span></div><span class="chev" aria-hidden="true">›</span></a>`;
    }).join('');
    if (showOthers) {
      return `<div class="page wrap">
        <div class="crumbs"><a href="#/browse/${kind}">${title}</a> › Other groups</div>
        <h1 class="title-xl">Other ${kind === 'techs' ? 'technology' : 'practice'} groups</h1>
        <div class="muted" style="margin:10px 0 22px;max-width:46rem">
          <p>Rulemart defines canonical “rule groups” like <code>${kind === 'techs' ? 'techs/go' : 'practices/testing'}</code> so that all rules from many user libraries can be aggregated in one place.</p>
          <p>All other groups that are not canonical are shown here. Their rules are searchable and appear on each library's page, but they aren't combined with rules from other libraries.</p>
        </div>
        ${otherRows ? rowList(otherRows) : '<div class="empty">None right now.</div>'}</div>`;
    }
    return `<div class="page wrap"><p class="index">Browse</p><h1 class="title-xl" style="margin:8px 0 0">${title}</h1>
      <nav class="tabs"><a href="#/browse/techs" class="${kind === 'techs' ? 'on' : ''}">Technologies</a><a href="#/browse/practices" class="${kind === 'practices' ? 'on' : ''}">Practices</a></nav>
      ${rowList(rows)}
      ${others.length ? `<p class="sm" style="margin-top:24px"><a class="faint" href="#/browse/${kind}/other">View other ${kind === 'techs' ? 'technology' : 'practice'} groups (${others.length}) →</a></p>` : ''}</div>`;
  }

  function librariesPage() {
    return `<div class="page wrap"><p class="index">Libraries</p><h1 class="title-xl" style="margin:8px 0 8px">Every public library on Rulemart</h1>
      <p class="muted" style="margin-bottom:22px">Anyone can add a public library. Check the owner line to see who stands behind each one.</p>
      ${rowList(libraries().map(libLine).join(''))}</div>`;
  }

  function groupPage(gid) {
    const g = D.groups[gid]; if (!g) return notFound();
    const { q } = parse();
    const f = readFilters(q);
    const sort = q.get('sort') || 'used';
    const base = allRules().filter(r => r.group === gid).map(r => ({ r, score: 1 }));
    const rows = applyFilters(base, f).sort(sorters[sort]);
    const libsN = new Set(base.map(x => x.r.lib.id)).size;
    const aliases = Object.keys(D.groups).filter(x => D.groups[x].similarTo === gid && allRules().some(r => r.group === x));
    const canon = isCanonical(gid);
    return `<div class="page wrap">
      <div class="crumbs"><a href="#/browse/${gid.startsWith('techs/') ? 'techs' : 'practices'}">${gid.startsWith('techs/') ? 'Technologies' : 'Practices'}</a>${canon ? '' : ` › <a href="#/browse/${gid.startsWith('techs/') ? 'techs' : 'practices'}/other">Other groups</a>`} › <span class="mono">${gid}</span></div>
      <h1 class="title-xl row" style="gap:14px">${techIcon(gid, 'lg')}${g.name}${canon ? '' : ' <span class="chip">Not canonical</span>'}</h1>
      ${canon ? '' : `<div class="note" style="margin-top:14px"><span><span class="mono">${gid}</span> isn't a canonical group, so it only includes rules from libraries that chose this exact name.${g.similarTo ? ` Looking for <a href="#/g/${g.similarTo}">${groupName(g.similarTo)}</a>?` : ''}</span></div>`}
      <p class="muted" style="margin:8px 0 24px">${base.length} ${base.length === 1 ? 'rule' : 'rules'} from ${libsN} ${libsN === 1 ? 'library' : 'libraries'}${groupBlurb(gid) ? ` · ${esc(groupBlurb(gid))}` : ''}</p>
      <div class="search-layout">${filterSidebar(base, f)}
        ${resultsList(rows, sort, [['used', 'Most used'], ['stars', 'Most starred'], ['new', 'Newest']], '', '<div class="empty">No rules match these filters. <button class="chip" data-clearfilters>Clear filters</button></div>')}
      </div>
      ${aliases.length ? `<p class="faint sm" style="margin-top:22px">Also see ${aliases.map(x => `<a href="#/g/${x}" class="mono">${x}</a>`).join(', ')}, a non-canonical group with similar rules.</p>` : ''}</div>`;
  }

  function ownerPage(login) {
    const o = D.owners[login]; if (!o) return notFound();
    const libs = libraries().filter(l => l.owner === login);
    return `<div class="page wrap">
      <div class="row" style="gap:18px;align-items:flex-start;flex-wrap:nowrap">${avatar(login, 'xl')}<div>
        <h1 class="title-xl">${esc(o.name)}</h1>
        <div class="meta" style="margin-top:6px"><span class="mono">github.com/${esc(login)}</span><span>${o.type === 'org' ? 'GitHub organization' : 'Personal account'}</span>${o.verified ? `<span class="verified">✓ ${o.verified}</span>` : ''}</div>
        <p class="muted" style="margin-top:8px">${esc(o.bio)}</p></div></div>
      <div class="sec-h" style="margin-top:30px"><span>Libraries</span><span>${libs.length}</span></div>
      <div style="margin-top:14px">${libs.length ? rowList(libs.map(libLine).join('')) : '<div class="empty">No libraries on Rulemart yet.</div>'}</div></div>`;
  }

  function libraryPage(id) {
    const lib = libById(id); if (!lib) return notFound();
    const { q } = parse();
    const tab = q.get('tab') || 'groups';
    const groupIds = [...new Set(lib.rules.map(r => r.group))];
    const disc = lib.rules.flatMap(r => discussionFor({ ...r, lib, key: `${lib.id}::${r.group}/${r.slug}` }).map(d => ({ ...d, rule: r })));
    const openDisc = disc.filter(d => d.state === 'open').length;
    const tabs = [['groups', 'Groups', groupIds.length], ['rules', 'All rules', lib.rules.length], ['releases', 'Releases', null], ['discussion', 'Discussion', openDisc]];
    if (isMaintainer(lib) && lib.insights) tabs.push(['insights', 'Insights', null]);
    let body = '';
    if (tab === 'groups') {
      const selected = (q.get('sel') ?? (lib.featuredSel || '')).split(',').filter(Boolean);
      const sel = selected.length ? selected : [];
      const row = g => { const n = lib.rules.filter(r => r.group === g).length; return `<label class="gsel"><input type="checkbox" data-gsel="${g}" ${sel.includes(g) ? 'checked' : ''}><div class="row" style="flex-wrap:nowrap;gap:10px">${techIcon(g)}<div><b>${groupName(g)}</b><span class="gid">${g}</span>${isCanonical(g) ? '' : ' <span class="flag" title="Not on the canonical list, so Rulemart won\'t combine it with other libraries">not canonical</span>'}</div></div><p>${esc(groupBlurb(g))}</p><span class="gc">${n} ${n === 1 ? 'rule' : 'rules'}</span></label>`; };
      const techs = groupIds.filter(g => g.startsWith('techs/'));
      const pracs = groupIds.filter(g => g.startsWith('practices/'));
      const cmd = sel.length ? importCommand(lib, sel) : '# Select one or more groups';
      body = `<div class="lib-cols"><div>
          ${techs.length ? `<p class="index list-label">Technologies · ${techs.length}</p>${rowList(techs.map(row).join(''))}` : ''}
          ${pracs.length ? `<p class="index list-label">Practices · ${pracs.length}</p>${rowList(pracs.map(row).join(''))}` : ''}
        </div>
        <aside><div class="adopt-box">
          <p class="index" style="margin-bottom:6px">Add to project</p>
          <p class="sm muted" style="margin-bottom:12px">${sel.length ? `${sel.length} ${sel.length === 1 ? 'group' : 'groups'} selected. Stays in sync with ${esc(libName(lib))}.` : 'Select groups to add. They stay in sync with this library.'}</p>
          <pre class="codebox">${esc(cmd)}</pre>
          <div class="row" style="margin-top:12px"><button class="btn small primary" data-copy="prompt-lib" ${sel.length ? '' : 'disabled'}>Copy prompt for agent</button><button class="btn small" data-copy="cmd-lib" ${sel.length ? '' : 'disabled'}>Copy command</button></div>
          <div class="row" style="margin-top:10px"><button class="chip" data-selall="1">Select all groups</button>${sel.length ? '<button class="chip" data-selall="0">Clear</button>' : ''}</div>
          ${sel.length ? `<details class="prompt"><summary>What the prompt says</summary><pre>${esc(libPrompt(lib, sel))}</pre></details>` : ''}
        </div></aside></div>`;
    } else if (tab === 'rules') {
      body = groupIds.map(g => `<div class="sec-h"><span>${groupName(g)} <span class="mono" style="text-transform:none;letter-spacing:0">${g}</span></span></div>${lib.rules.filter(r => r.group === g).map(r => ruleResult({ ...r, lib, key: `${lib.id}::${r.group}/${r.slug}` }, '', { showGroup: false })).join('')}`).join('');
    } else if (tab === 'releases') {
      const entries = lib.rules.flatMap(x => { const rr = { ...x, lib, key: `${lib.id}::${x.group}/${x.slug}` }; return ruleVersions(rr).map(v => ({ ...v, r: rr })); });
      const byRelease = [...new Set(entries.map(e => e.i))].sort((a, b) => b - a);
      body = byRelease.map(i => `<p class="index list-label">${esc(lib.tags[i].date)}</p>${rowList(entries.filter(e => e.i === i).map(e => `<a class="rowlink" href="${ruleUrl(e.r)}?tab=versions"><div><div class="t">${esc(e.r.title)}</div><div class="s">${esc(e.summary)}</div></div><span class="mono sm">${e.version}</span><span class="chev" aria-hidden="true">›</span></a>`).join(''))}`).join('')
        + '<p class="faint sm" style="margin-top:16px">Every rule is versioned on its own. Each entry is a tag like <span class="mono">practices/testing/verify-retry-limits@1.3.0</span>.</p>';
    } else if (tab === 'discussion') {
      body = discussionList(disc, lib, true);
    } else if (tab === 'insights') {
      body = insightsView(lib);
    }
    const groupsN = groupIds.length;
    return `<div class="page wrap">
      <div class="libhead">${avatar(lib.owner, 'lg')}<div>
        <h1 class="title-xl" style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">${esc(libName(lib))}</h1>
        <p class="muted" style="margin:6px 0 8px">${esc(lib.description)}</p>
        <div class="meta"><span class="mono">${esc(lib.id)}</span><span>${esc(lib.license)}</span><span>${lib.rules.length} rules</span><span>${lib.usedBy ? `Used by ${fmt(lib.usedBy)} public projects` : 'Not used by public projects yet'}</span><span>Updated ${lib.tags[lib.tags.length - 1].date}</span></div></div>
        <div class="row actions"><a class="btn small" href="#" data-act="noop">${icon.gh}View on GitHub</a></div></div>
      ${originCards(lib)}
      <nav class="tabs">${tabs.map(([k, l, n]) => `<a href="${libUrl(lib, k)}" class="${tab === k ? 'on' : ''}">${l}${n !== null ? `<span class="n">${n}</span>` : ''}</a>`).join('')}</nav>
      ${body}</div>`;
  }

  function insightsView(lib) {
    const featured = lib.rules.find(r => r.removals);
    return `<div class="note" style="margin-bottom:18px"><span>Visible to maintainers of ${esc(lib.id)} only. Counts come from the Git history of public projects.</span></div>
      <table class="data"><thead><tr><th>Rule</th><th>Using now</th><th>Ever added</th><th>Removed</th><th>Most common reason</th></tr></thead><tbody>
      ${lib.insights.map(i => { const r = lib.rules.find(x => x.slug === i.rule); return `<tr><td><a href="${ruleUrl({ ...r, lib })}">${esc(r.title)}</a></td><td>${fmt(i.using)}</td><td>${fmt(i.ever)}</td><td>${fmt(i.removed)}</td><td>${esc(i.reason)}</td></tr>`; }).join('')}
      </tbody></table>
      ${featured ? `<p class="index" style="margin:28px 0 10px">Recent removals of “${esc(featured.title)}”</p>${featured.removals.map(x => `<div class="reason">${x.reason ? `“${esc(x.reason)}”` : '<span class="muted">No reason recorded. The project stopped importing the group.</span>'}<div class="by"><span class="kind">${x.kind}</span>${esc(x.project)} · ${x.when} · at ${x.at}${x.with ? ` · with <span class="mono">${esc(x.with)}</span>` : ''} · <a href="#" data-act="noop">view commit</a></div></div>`).join('')}` : ''}`;
  }

  function discussionList(items, lib, showRule) {
    if (!items.length) return `<div class="empty">No discussion yet. Issues and pull requests about ${showRule ? 'these rules' : 'this rule'} on GitHub will show up here.</div>`;
    const prs = items.filter(d => d.kind === 'pr');
    const issues = items.filter(d => d.kind === 'issue');
    const item = d => `<div class="disc-item"><div><a class="tt" href="#/gh/issue/${lib.id}/${d.num}">${esc(d.title)}</a>${d.mine ? ' <span class="chip on" style="margin-left:4px">Yours</span>' : ''}
      <div class="s"><span>#${d.num}</span><span>· by ${esc(d.author)}</span>${d.relation ? `<span>· <b style="color:var(--ink);font-weight:500">${d.relation === 'changes' ? 'changes this rule' : 'mentions this rule'}</b></span>` : ''}<span>· ${d.comments} comments</span>${d.note ? `<span>· ${esc(d.note)}</span>` : ''}${d.last ? `<span>· last: “${esc(d.last)}”</span>` : ''}${showRule ? `<span>· <a href="${ruleUrl({ ...d.rule, lib })}">${esc(d.rule.title)}</a></span>` : ''}</div></div>
      ${d.state === 'open' ? '<span class="pill open">Open</span>' : `<span class="pill">Closed${d.closedIn ? ` · fixed in ${d.closedIn}` : ''}</span>`}</div>`;
    return `${prs.length ? `<div class="sec-h"><span>Pull requests</span><span>${prs.length}</span></div>${prs.map(item).join('')}` : ''}
      ${issues.length ? `<div class="sec-h"><span>Issues</span><span>${issues.length}</span></div>${issues.map(item).join('')}` : ''}
      <p class="faint xs" style="margin-top:14px">Mirrored from GitHub. Everything opens on GitHub; Rulemart stores no comments.</p>`;
  }

  function rulePage(parts) {
    const [owner, repo, kind, g, slug] = parts;
    const lib = libById(`${owner}/${repo}`); if (!lib) return notFound();
    const base = lib.rules.find(r => r.group === `${kind}/${g}` && r.slug === slug); if (!base) return notFound();
    const r = { ...base, lib, key: `${lib.id}::${base.group}/${base.slug}` };
    const { q } = parse();
    const tab = q.get('tab') || 'rule';
    const disc = discussionFor(r);
    const openN = disc.filter(d => d.state === 'open').length;
    const versions = ruleVersions(r);
    const starred = !!state.stars[r.key];
    let body = '';
    if (tab === 'rule') {
      body = `<div class="rule-cols"><div class="md">
          <div class="whento"><b>When to apply</b>${esc(r.whenToRead)}</div>${r.body.replace(/<pre><code>/g, `<pre><code class="language-${ruleLang(r)}">`)}</div>
        <aside class="side">
          <div class="panel"><div class="panel-h"><span class="index">Usage</span></div><div class="panel-b">
            <div class="stat"><div class="big">${fmt(r.usedBy)}</div><div class="lbl">public projects use this rule</div><div class="sub">${r.usedBy ? `Net ${r.net30 >= 0 ? '+' : ''}${r.net30} in the last 30 days` : 'New on Rulemart'}</div></div>
            <div class="stat"><div class="big">${openN}</div><div class="lbl">open issues and PRs</div><div class="sub"><a href="${ruleUrl(r)}?tab=discussion">See the discussion</a></div></div></div></div>
          <div class="kv">
            <div><span>Owner</span><span><a href="#/o/${lib.owner}">${esc(lib.owner)}</a></span></div>
            <div><span>Repository</span><span><a class="mono" href="https://github.com/${esc(lib.id)}" data-act="ghlink">${esc(lib.id.split('/')[1])}</a></span></div>
            <div><span>License</span><span>${esc(lib.license)}</span></div>
            <div><span>File</span><span><a class="mono" href="https://github.com/${esc(lib.id)}/blob/${r.group}/${r.slug}@${ruleVersion(r)}/${r.group}/${r.slug}.md" data-act="ghlink" title="${r.group}/${r.slug}.md">${r.slug}.md</a></span></div>
          </div></aside></div>`;
    } else if (tab === 'discussion') {
      body = `<div class="row between" style="margin-bottom:10px"><span class="muted sm">Issues and pull requests on <span class="mono">${esc(lib.id)}</span> that are about this rule.</span><button class="btn small" data-act="discuss">${icon.chat}Discuss</button></div>${discussionList(disc, lib, false)}`;
    } else if (tab === 'usedby' && r.usedBy === 0) {
      body = '<div class="empty">No public projects use this rule yet. Rulemart checks new projects every day.</div>';
    } else if (tab === 'usedby') {
      const pool = D.projectPool;
      const n = Math.min(pool.length, 10);
      body = `<p class="muted sm" style="margin-bottom:12px">Public projects whose <span class="mono">.code-rules/generated/provenance.json</span> lists this rule.</p>
        ${rowList(pool.slice(0, n).map((p, i) => `<a class="rowlink" href="https://github.com/${esc(p)}" data-act="ghlink"><div class="row" style="flex-wrap:nowrap;gap:12px">${avatar(p.split('/')[0], 'md')}<div><div class="t">${esc(p)}</div><div class="s">on ${versions[Math.min(versions.length - 1, i % 3)].version}</div></div></div><span class="faint sm">synced ${i + 2} days ago</span><span class="chev" aria-hidden="true">›</span></a>`).join(''))}
        <p class="faint sm" style="margin-top:14px">and ${fmt(Math.max(0, r.usedBy - n))} more public projects</p>`;
    } else if (tab === 'versions') {
      body = `<p class="faint sm" style="margin:0 0 14px">Versions follow semver for rules: <b>major</b> changes what the rule requires, <b>minor</b> widens its guidance, <b>patch</b> clarifies wording or examples.</p>`
        + rowList(versions.map((v, k) => `<div class="vrow"><div><div class="row" style="gap:10px"><b class="mono">${v.version}</b>${versionChips(v, k === 0)}<span class="faint sm">${esc(v.date)}</span></div><div class="muted sm" style="margin-top:4px">${esc(v.summary)}</div></div>${v.diff ? `<a class="sm" href="#" data-act="noop">View diff</a>` : '<span></span>'}</div>`).join(''));
    }
    const tabs = [['rule', 'Rule', null], ['discussion', 'Discussion', openN], ['usedby', 'Used by', fmt(r.usedBy)], ['versions', 'Versions', versions.length]];
    return `<div class="page wrap">
      <div class="crumbs">${avatar(lib.owner)}<a href="${libUrl(lib)}">${esc(libName(lib))}</a> › ${techIcon(r.group, 'xs')}<a href="#/g/${r.group}">${groupName(r.group)}</a> <span class="mono">${r.group}</span></div>
      <div class="rulehead"><div>
        <h1 class="title-xl">${esc(r.title)}</h1>
        <div class="meta" style="margin-top:10px">${impact(r.impact)}<span class="mono" title="Latest version">${ruleVersion(r)}</span>${r.tags.map(t => `<a class="tag" href="#/search?q=${encodeURIComponent(t)}" title="Search tag">#${esc(t)}</a>`).join('')}</div>
        <div class="engage">
          <button class="btn small ghost ${starred ? 'on' : ''}" data-act="star" aria-pressed="${starred}">${starred ? icon.starOn : icon.star}${starred ? 'Starred' : 'Star'} <span class="faint">${fmt(starCount(r))}</span></button>
          <button class="btn small ghost" data-act="discuss">${icon.chat}Discuss <span class="faint">${openN}</span></button>
        </div></div>
        <button class="btn primary" data-act="add-project">Add to project</button></div>
      <nav class="tabs">${tabs.map(([k, l, n]) => `<a href="${ruleUrl(r)}${k === 'rule' ? '' : `?tab=${k}`}" class="${tab === k ? 'on' : ''}">${l}${n !== null ? `<span class="n">${n}</span>` : ''}</a>`).join('')}</nav>
      ${body}</div>`;
  }

  // ---------- Dashboard ----------
  function myLibraryIds() {
    const pub = libraries().filter(l => l.owner === user.login || user.orgs.includes(l.owner)).map(l => l.id);
    const used = projects().flatMap(p => p.sources.map(s => s.lib));
    return [...new Set([...pub, ...used])];
  }
  const projects = () => [...D.myProjects.public.map(p => ({ ...p, private: false })), ...(state.private ? D.myProjects.private.map(p => ({ ...p, private: true })) : [])];

  function me() {
    if (!state.signedIn) return signinPage();
    const { q } = parse();
    const tab = q.get('tab') || 'libraries';
    let body = '';
    if (tab === 'libraries') {
      const published = libraries().filter(l => l.owner === user.login || user.orgs.includes(l.owner));
      const usedMap = {};
      projects().forEach(p => p.sources.forEach(s => { (usedMap[s.lib] = usedMap[s.lib] || []).push({ ...s, repo: p.repo, private: p.private }); }));
      body = `${state.private
        ? `<div class="note"><span>Including private projects from the repos you selected. Private projects are only visible to you.</span><a href="#/me/private">Manage</a></div>`
        : `<div class="note"><span>Showing public repos only.</span><a href="#/me/private">Include private projects</a></div>`}
        <div class="sec-h" style="margin-top:24px"><span>Published by you and your orgs</span><span>${published.length}</span></div>
        ${published.map(l => `<div class="list-row"><div><div class="t"><a href="${libUrl(l)}">${esc(libName(l))}</a>${l.fresh ? ' <span class="chip on">New</span>' : ''}</div><div class="s"><span class="mono">${esc(l.id)}</span> · ${l.rules.length} rules</div></div>
          <div class="row"><span class="meta"><span>${usedBy(l.usedBy)}</span></span>${l.insights ? `<a class="btn small" href="${libUrl(l, 'insights')}">Insights</a>` : ''}</div></div>`).join('')}
        <div style="margin-top:12px"><a class="btn small" href="#/me/add">+ Add a library</a></div>
        <div class="sec-h" style="margin-top:30px"><span>Used in your projects</span><span>${Object.keys(usedMap).length}</span></div>
        ${Object.entries(usedMap).map(([id, uses]) => { const l = libById(id); return `<div class="list-row"><div><div class="t"><a href="${libUrl(l)}">${esc(libName(l))}</a></div>
          <div class="s">${uses.map(u => `${esc(u.repo)}${u.private ? ' (private)' : ''}${u.updates ? ` · <b style="color:var(--ink);font-weight:500">${u.updates} rule ${u.updates === 1 ? 'update' : 'updates'}</b>` : ' · up to date'}`).join(' · ')}</div></div>
          <span class="meta"><span>${uses.length} ${uses.length === 1 ? 'project' : 'projects'}</span></span></div>`; }).join('')}
        <p class="faint xs" style="margin-top:12px">Read from each project's <span class="mono">.code-rules/generated/provenance.json</span>.</p>`;
    } else {
      const starred = allRules().filter(r => state.stars[r.key]);
      body = starred.length ? starred.map(r => ruleResult(r)).join('') : '<div class="empty">You haven\'t starred any rules yet. Star a rule from its page.</div>';
    }
    return `<div class="page wrap">
      <div class="row" style="gap:16px">${avatar(user.login, 'lg')}<div><p class="index">Dashboard</p><h1 class="title-xl">${user.name}</h1><div class="faint sm">@${user.login} · member of fabricahq</div></div></div>
      <nav class="tabs"><a href="#/me" class="${tab === 'libraries' ? 'on' : ''}">My libraries</a><a href="#/me?tab=stars" class="${tab === 'stars' ? 'on' : ''}">Starred rules<span class="n">${Object.keys(state.stars).length}</span></a></nav>
      ${body}</div>`;
  }

  function addLibraryPage() {
    if (!state.signedIn) return signinPage();
    const { q } = parse();
    const err = q.get('err');
    const onHub = libraries().filter(l => l.owner === user.login || user.orgs.includes(l.owner));
    const candidates = D.publishable.filter(p => p.visibility === 'public' || state.private);
    const rows = [
      ...candidates.map(p => {
        if (p.visibility === 'private') return `<div class="pick dim"><div><b>${esc(p.id)}</b><div class="xs faint">Private · ${p.groups} groups</div></div><span class="sm faint">Private libraries can't be published on Rulemart</span></div>`;
        if (state.added.includes(p.id)) return `<div class="pick"><div><b>${esc(p.id)}</b><div class="xs faint">Public</div></div><a class="chip on" href="${libUrl(libById(p.id))}">✓ On Rulemart</a></div>`;
        return `<div class="pick"><div><b>${esc(p.id)}</b><div class="xs faint">Public · ${p.groups} group</div></div><a class="btn small primary" href="#/me/add/run?repo=${encodeURIComponent(p.id)}">Add this library</a></div>`;
      }),
      ...onHub.filter(l => !l.fresh).map(l => `<div class="pick"><div><b>${esc(l.id)}</b><div class="xs faint">Public${l.owner !== user.login ? ` · via the ${esc(l.owner)} organization` : ''}</div></div><a class="chip on" href="${libUrl(l)}">✓ On Rulemart</a></div>`),
    ];
    return `<div class="page wrap" style="max-width:780px">
      <p class="index">Publish</p><h1 class="title-xl" style="margin:8px 0 6px">Add a library</h1>
      <p class="muted" style="margin-bottom:20px">Your repos and your organizations' repos that contain a <span class="mono">rule-library.yaml</span>. The library's page will show its owner, its repository, and that you added it.</p>
      <div class="panel"><div class="panel-h"><span class="index">Your libraries on GitHub</span></div><div class="panel-b">${rows.join('')}</div></div>
      <div class="note" style="margin-top:14px"><span>Only public libraries can be published on Rulemart.${state.private ? '' : ' Rulemart can only see your public repos right now.'}</span>${state.private ? '' : '<a href="#/me/private">Include private repos</a>'}</div>
      <form data-form="addurl" style="margin-top:28px"><p class="index" style="margin-bottom:8px">Or add any public library by URL</p>
        <div class="row" style="flex-wrap:nowrap"><input class="input" name="url" placeholder="https://github.com/owner/repo" autocomplete="off"><button class="btn" type="submit">Add</button></div>
        ${err ? `<p class="err">${esc(err)}</p>` : '<p class="faint xs" style="margin-top:8px">Anyone can add a public library. It will show that you added it.</p>'}
      </form></div>`;
  }

  function addRunPage() {
    const { q } = parse();
    const repo = q.get('repo');
    const p = D.publishable.find(x => x.id === repo);
    if (!p) return notFound();
    const lib = p.library;
    const steps = [`Found <b>rule-library.yaml</b> in ${esc(repo)}`, `Library check passed`, `<b>${new Set(lib.rules.map(r => r.group)).size} group</b>, <b>${lib.rules.length} rules</b> indexed`, `Tagged ${lib.rules.length} rules at <b>1.0.0</b> · license <b>${lib.license}</b>`, 'Watching for new rule releases'];
    setTimeout(() => runSteps(repo, steps.length), 50);
    return `<div class="page wrap" style="max-width:640px">
      <p class="index">Publish</p><h1 class="title-xl" style="margin:8px 0 18px">Adding ${esc(repo)}</h1>
      <div class="panel"><div class="panel-b">${steps.map((s, i) => `<div class="check" data-step="${i}"><span class="tick">✓</span><span>${s}</span></div>`).join('')}</div></div>
      <div id="run-done" class="hidden" style="margin-top:20px"><p style="font-size:17px;font-weight:500;margin-bottom:6px">${esc(repo)} is live on Rulemart.</p><p class="muted sm">Push a new tag and Rulemart updates within minutes.</p>
        <div class="row" style="margin-top:14px"><a class="btn primary" href="${`#/l/${repo}`}">View library page</a><a class="btn" href="#/me">Back to Dashboard</a></div></div></div>`;
  }
  function runSteps(repo, n) {
    let i = 0;
    const tick = () => {
      const prev = document.querySelector(`[data-step="${i - 1}"]`); if (prev) { prev.classList.remove('run'); prev.classList.add('done'); }
      if (i === n) {
        if (!state.added.includes(repo)) { state.added.push(repo); save(); }
        $('#run-done')?.classList.remove('hidden'); return;
      }
      const cur = document.querySelector(`[data-step="${i}"]`); if (!cur) return;
      cur.classList.add('run'); i += 1; setTimeout(tick, 650);
    };
    tick();
  }

  function privatePage() {
    if (!state.signedIn) return signinPage();
    return `<div class="page wrap" style="max-width:820px">
      <span class="chip">Optional</span>
      <h1 class="title-xl" style="margin:12px 0 8px">Include your private projects?</h1>
      <p class="muted" style="margin-bottom:22px">Rulemart already sees your public repos. To also list private projects that use Code Rules, install the Rulemart GitHub App on the repos you choose. GitHub will ask you to approve these permissions:</p>
      <table class="data"><thead><tr><th>On GitHub</th><th>Why we ask</th><th>If you don't grant it</th></tr></thead><tbody>
        <tr><td>Contents: read-only</td><td>To read <span class="mono">.code-rules/generated/provenance.json</span> and <span class="mono">rule-library.yaml</span>, so we can see which of your private projects use Code Rules and which libraries they import.</td><td>We only look at your public repos.</td></tr>
        <tr><td>Metadata: read-only</td><td>GitHub requires this for every app. It lets us list the repos you choose.</td><td>Only applies if you install.</td></tr>
      </tbody></table>
      <div class="grid2" style="margin-top:20px">
        <div class="card"><b>We never</b><ul class="muted sm" style="margin:8px 0 0;padding-left:18px"><li>Write to your repos, or open issues or PRs</li><li>Store your code. We keep library names, versions, and group IDs</li><li>Show private projects to anyone else, or count them in public numbers</li></ul></div>
        <div class="card"><b>Worth knowing</b><p class="muted sm" style="margin:8px 0 0">GitHub can't limit an app to one folder, so “Contents” technically covers each repo you select. Rulemart only reads the two files above. Choose <b>Only select repositories</b> on GitHub to keep the scope small.</p></div>
      </div>
      <div class="row" style="margin-top:26px">${state.private
        ? `<a class="btn" href="#/me">Done</a><button class="btn" data-act="disconnect">Remove access to private repos</button>`
        : `<a class="btn primary" href="#/gh/install">${icon.gh}Continue to GitHub</a><a class="btn" href="#/me" data-act="skip-private">Skip, public repos only</a>`}
        <span class="faint sm">You can change this anytime.</span></div></div>`;
  }

  function signinPage() {
    const perks = [
      'Star the rules you find useful',
      'Track the libraries your projects use',
      'Publish your libraries and see who uses them',
    ];
    return `<div class="page wrap" style="max-width:480px;padding-top:72px">
      <div style="width:44px;height:44px;margin:0 auto;color:var(--ink)">${icon.fab}</div>
      <h1 class="title-xl" style="margin:18px 0 22px;text-align:center">Sign in to Rulemart</h1>
      <p style="margin:0 0 14px;font-weight:500">With a free Rulemart account, you can:</p>
      <ul class="perks">${perks.map(p => `<li><span class="tick">✓</span>${p}</li>`).join('')}</ul>
      <a class="btn primary" style="width:100%;margin-top:26px" href="#/gh/authorize">${icon.gh}Continue with GitHub</a>
      <p class="faint sm" style="margin-top:14px;text-align:center">Browsing needs no account. Rulemart reads your public profile and public repos, and never writes to GitHub.</p></div>`;
  }

  function notFound() {
    return `<div class="page wrap"><h1 class="title-xl">Not found</h1><p class="muted" style="margin-top:10px">That page isn't in the mock. <a href="#/">Go home</a>.</p></div>`;
  }

  // ---------- Simulated GitHub ----------
  const ghShell = (path, inner) => `<div class="sim-banner">Simulated GitHub page. In the real product, this is github.com.</div>
    <div class="gh"><div class="gh-top">${icon.gh}<span class="path">${path}</span></div><div class="gh-main">${inner}</div></div>`;

  function ghAuthorize() {
    return ghShell('Authorize application', `<div style="max-width:460px;margin:20px auto;text-align:center">
      <div class="row" style="justify-content:center;gap:16px;margin-bottom:18px"><span class="avatar lg org">${icon.fab.replace('<svg', '<svg style="width:28px;height:28px"')}</span><span class="muted">⋯</span>${avatar(user.login, 'lg')}</div>
      <h2 style="font-size:24px;font-weight:400;margin-bottom:18px">Authorize Rulemart</h2>
      <div class="gh-box" style="text-align:left"><div class="gh-box-b">
        <p style="margin-bottom:10px"><b>Rulemart</b> by <b>fabricahq</b> wants to access your <b>${user.login}</b> account</p>
        <div class="gh-perm"><span>👤</span><div><b>Verify your GitHub identity</b><div class="muted">Read your username and public profile</div></div></div>
        <div class="gh-perm"><span>📦</span><div><b>Public repositories</b><div class="muted">Read-only access to public repos and org memberships</div></div></div>
      </div></div>
      <div class="row" style="justify-content:center;margin-top:18px"><button class="gh-btn alt" data-act="gh-cancel">Cancel</button><button class="gh-btn" data-act="gh-authorize">Authorize fabricahq</button></div>
      <p class="muted" style="font-size:12px;margin-top:14px">Authorizing will redirect to rulemart.fabricahq.com</p></div>`);
  }

  function ghInstall() {
    const repos = ['josh-padnick/billing-service', 'josh-padnick/team-rules', 'josh-padnick/mobile-app', 'josh-padnick/notes'];
    return ghShell('Install Rulemart', `<div style="max-width:620px;margin:0 auto">
      <h2 style="font-size:24px;font-weight:400;margin-bottom:16px">Install Rulemart</h2>
      <div class="gh-box"><div class="gh-box-h">Install on your personal account <b>${user.login}</b></div><div class="gh-box-b">
        <label class="row" style="margin-bottom:8px"><input type="radio" name="scope"> <b>All repositories</b></label>
        <label class="row"><input type="radio" name="scope" checked> <b>Only select repositories</b></label>
        <div style="margin:10px 0 0 24px">${repos.map((r, i) => `<label class="row" style="margin:4px 0"><input type="checkbox" ${i < 3 ? 'checked' : ''}> ${r} <span class="muted" style="font-size:12px">Private</span></label>`).join('')}</div>
      </div>
      <div class="gh-box-b" style="border-top:1px solid var(--gh-border)"><b>with these permissions:</b>
        <div class="gh-perm"><span>✓</span><div>Read access to code and metadata</div></div></div></div>
      <div class="row" style="margin-top:16px"><button class="gh-btn" data-act="gh-install">Install</button><button class="gh-btn alt" data-act="gh-cancel-install">Cancel</button></div></div>`);
  }

  const feedbackTopics = {
    format: { label: 'The rule format', ex: 'Frontmatter fields, groups, impact levels, how rules are written.' },
    cli: { label: 'The CLI', ex: 'Commands, sync and update, error messages, config.' },
    agents: { label: 'How agents use rules', ex: "Rules your agent missed, misread, or applied where it shouldn't." },
    hub: { label: 'Rulemart', ex: 'Search, library and rule pages, anything missing.' },
    docs: { label: 'Documentation', ex: 'Anything unclear, wrong, or hard to find.' },
    other: { label: 'Something else', ex: "Ideas, questions, or anything that doesn't fit above." },
  };

  function ghNewIssue() {
    const { q } = parse();
    const repo = q.get('repo');
    const ruleKey = q.get('rule');
    const topic = q.get('topic');
    const r = ruleKey ? allRules().find(x => x.key === ruleKey) : null;
    let title = ''; let bodyText = '';
    if (r) {
      title = `[${r.title}] `;
      bodyText = `**Rule:** ${r.group}/${r.slug}@${ruleVersion(r)}\n\n<!-- Share your experience, ask a question, or suggest a change. -->\n\n\n---\nOpened from Rulemart`;
    } else if (topic) {
      const t = feedbackTopics[topic];
      title = `[${t.label}] `;
      bodyText = `**Topic:** ${t.label}\n\n**What happened, or what would you change?**\n\n\n**Why it matters to you:**\n\n\n---\nOpened from Rulemart`;
    }
    return ghShell(`${esc(repo)} / Issues / New`, `<h2 style="font-size:22px;font-weight:400;margin-bottom:16px">Create new issue</h2>
      <form data-form="ghissue" class="gh-box"><div class="gh-box-b">
        <input type="hidden" name="repo" value="${esc(repo)}"><input type="hidden" name="rule" value="${esc(ruleKey || '')}"><input type="hidden" name="topic" value="${esc(topic || '')}">
        <label style="font-weight:600;display:block;margin-bottom:6px">Add a title</label><input class="gh-in" name="title" value="${esc(title)}" required>
        <label style="font-weight:600;display:block;margin:14px 0 6px">Add a description</label><textarea name="body">${esc(bodyText)}</textarea>
        <div class="row" style="justify-content:space-between;margin-top:12px"><span class="muted" style="font-size:12px">Labels: ${r ? 'rulemart' : `feedback, ${topic}`}</span>
          <div class="row"><button type="button" class="gh-btn alt" data-act="gh-back">Cancel</button><button class="gh-btn" type="submit">Submit new issue</button></div></div>
      </div></form>`);
  }

  function ghIssue(parts) {
    const [owner, repo, numS] = parts;
    const id = `${owner}/${repo}`;
    const num = Number(numS);
    const mine = state.issues.find(i => i.repo === id && i.num === num);
    let item = mine;
    let ruleKey = mine?.ruleKey;
    if (!item) {
      const lib = libById(id);
      lib?.rules.forEach(r => (r.discussion || []).forEach(d => { if (d.num === num) { item = { ...d, body: `Opened ${d.when}.`, repo: id }; ruleKey = `${lib.id}::${r.group}/${r.slug}`; } }));
    }
    if (!item) return ghShell(esc(id), '<p>Issue not found in the mock.</p>');
    const r = ruleKey ? allRules().find(x => x.key === ruleKey) : null;
    const closed = item.state === 'closed';
    const comments = mine ? [] : [
      { who: `${owner}-maintainer`, text: item.kind === 'pr' ? 'Thanks! Reviewing the proposed wording now.' : 'Thanks for raising this. Could you share an example of where it came up?' },
      { who: item.author, text: item.kind === 'pr' ? 'Updated the examples based on the review.' : 'Sure. I added a short example below and can help test a fix.' },
    ];
    return ghShell(`${esc(id)} / ${item.kind === 'pr' ? 'Pull requests' : 'Issues'} / #${num}`, `
      <h2 style="font-size:28px;font-weight:400">${esc(item.title)} <span class="muted">#${num}</span></h2>
      <div class="row" style="margin:10px 0 18px"><span class="gh-state ${closed ? 'closed' : ''}">${closed ? 'Closed' : 'Open'}</span><span class="muted"><b>${esc(item.author)}</b> opened this ${esc(item.when || 'just now')}</span></div>
      ${r ? `<div class="gh-box" style="padding:10px 14px;margin-bottom:6px;font-size:13px">About rule <b>${esc(r.group)}/${esc(r.slug)}</b> · <a href="${ruleUrl(r)}?tab=discussion">View on Rulemart</a></div>` : ''}
      <div class="gh-comment"><div class="h"><b>${esc(item.author)}</b> commented</div><div class="b">${esc(item.body || '')}</div></div>
      ${comments.map(cm => `<div class="gh-comment"><div class="h"><b>${esc(cm.who)}</b> commented</div><div class="b">${esc(cm.text)}</div></div>`).join('')}
      <div class="row" style="margin-top:20px"><button class="gh-btn alt" data-act="gh-back-hub" data-href="${r ? `${ruleUrl(r)}?tab=discussion` : '#/'}">← Back to Rulemart</button></div>`);
  }

  // ---------- Commands and prompts ----------
  function importCommand(lib, groupIds) {
    const lines = [`code-rules project add library ${alias(lib)} \\`, `  --repository https://github.com/${lib.id}.git \\`];
    groupIds.forEach((g, i) => lines.push(`  --groups ${g}${i < groupIds.length - 1 ? ' \\' : ''}`));
    lines.push('code-rules project sync');
    return lines.join('\n');
  }
  const forkCommand = r => [`code-rules project add rule \\`, `  ${r.group}/${r.slug} \\`, `  --from ${r.lib.id}@${ruleVersion(r)}`, 'code-rules project build'].join('\n');
  const installNote = 'If the code-rules CLI is not installed, install it first: https://code-rules.fabricahq.com/start-here/install/';
  const libPrompt = (lib, groupIds) => `Add these rule groups from the ${lib.id} Code Rules library to this project and keep them in sync with the library: ${groupIds.join(', ')}.\n\nRun:\n${importCommand(lib, groupIds)}\n\n${installNote}\nThen make sure AGENTS.md tells agents to read .code-rules/generated/RULES.md.`;
  const groupPrompt = r => `Add the "${groupName(r.group)}" rule group (${r.group}) from the ${r.lib.id} Code Rules library to this project, and keep it in sync with the library. It includes the rule "${r.title}".\n\nRun:\n${importCommand(r.lib, [r.group])}\n\n${installNote}\nThen make sure AGENTS.md tells agents to read .code-rules/generated/RULES.md.`;
  const forkPrompt = r => `Fork the Code Rules rule "${r.title}" (${r.group}/${r.slug}) from ${r.lib.id}, version ${ruleVersion(r)}, into this project's local rules, with attribution. We will edit it ourselves, so it should not track upstream updates.\n\nRun:\n${forkCommand(r)}\n\n${installNote}`;

  // ---------- Modals ----------
  function openModal(html, narrow) {
    const root = $('#modal-root');
    root.innerHTML = `<div class="scrim" data-act="close-scrim"><div class="modal ${narrow ? 'narrow' : ''}" role="dialog" aria-modal="true">${html}</div></div>`;
    root.querySelector('.modal button, .modal a')?.focus();
  }
  const closeModal = () => { $('#modal-root').innerHTML = ''; };
  const currentRule = () => { const { parts } = parse(); if (parts[0] !== 'r') return null; const [owner, repo, kind, g, slug] = parts.slice(1); return allRules().find(x => x.lib.id === `${owner}/${repo}` && x.group === `${kind}/${g}` && x.slug === slug); };

  function addProjectModal(r) {
    const n = r.lib.rules.filter(x => x.group === r.group).length;
    openModal(`<div class="modal-h"><div><h2>Add to project</h2><p class="muted sm" style="margin:4px 0 0">${esc(r.title)} · ${esc(libName(r.lib))}</p></div><button class="x" data-act="close" aria-label="Close">×</button></div>
      <div class="option lead"><div class="row between"><h3>1. Add the ${groupName(r.group)} group</h3><span class="chip on">Stays in sync</span></div>
        <ul><li>${n} ${n === 1 ? 'rule' : 'rules'} from ${esc(libName(r.lib))}, including this one</li><li>Updates arrive when you run sync</li>${n > 1 ? `<li>Don't want some of the other ${n - 1}? Exclude them in your config</li>` : ''}</ul>
        <pre class="codebox">${esc(importCommand(r.lib, [r.group]))}</pre>
        <div class="row"><button class="btn small primary" data-copy="prompt-group">Copy prompt for agent</button><button class="btn small" data-copy="cmd-group">Copy command</button></div>
        <details class="prompt"><summary>What the prompt says</summary><pre>${esc(groupPrompt(r))}</pre></details></div>
      <div class="option"><div class="row between"><h3>2. Fork only this rule</h3><span class="chip">You own it</span></div>
        <p class="muted sm" style="margin:0">Copies just this rule into <span class="mono">.code-rules/local/</span>, with attribution to version ${ruleVersion(r)}. Edit it freely; it won't get updates from ${esc(r.lib.owner)}.</p>
        <pre class="codebox">${esc(forkCommand(r))}</pre>
        <div class="row"><button class="btn small primary" data-copy="prompt-fork">Copy prompt for agent</button><button class="btn small" data-copy="cmd-fork">Copy command</button><span class="flag">--from is a proposed CLI flag</span></div>
        <details class="prompt"><summary>What the prompt says</summary><pre>${esc(forkPrompt(r))}</pre></details></div>`);
  }

  function discussModal(r) {
    const open = discussionFor(r).filter(d => d.state === 'open');
    const newIssue = `#/gh/new?repo=${encodeURIComponent(r.lib.id)}&rule=${encodeURIComponent(r.key)}`;
    openModal(`<div class="modal-h"><div><h2>Discuss this rule</h2><p class="muted sm" style="margin:4px 0 0">Share an experience, ask a question, or suggest a change. Discussions are GitHub issues on the library's repo.</p></div><button class="x" data-act="close" aria-label="Close">×</button></div>
      ${open.length ? `<p class="index" style="margin:14px 0 2px">Already open</p>${open.slice(0, 4).map(d => `<a class="disc-link" href="#/gh/issue/${r.lib.id}/${d.num}" data-act="close-nav"><div><span class="tt">${esc(d.title)}</span><div class="s"><span class="kind">${d.kind === 'pr' ? 'PR' : 'Issue'}</span><span>#${d.num}</span><span>· ${d.comments} comments</span></div></div><span class="chev" aria-hidden="true">›</span></a>`).join('')}` : ''}
      <div style="margin-top:20px"><a class="btn primary" href="${newIssue}" data-act="close-nav">${icon.gh}Start a discussion on GitHub</a></div>`, true);
  }

  function feedbackModal() {
    openModal(`<div class="modal-h"><div><h2>Help shape Code Rules</h2><p class="muted sm" style="margin:4px 0 0">Code Rules is young, and we're changing it based on what you tell us. Pick a topic to open an issue on GitHub.</p></div><button class="x" data-act="close" aria-label="Close">×</button></div>
      <div class="grid2">${Object.entries(feedbackTopics).map(([k, t]) => `<a class="tile" href="#/gh/new?repo=fabricahq/code-rules&topic=${k}" data-act="close-nav"><div class="row between"><b>${t.label}</b><span class="faint xs">↗ GitHub</span></div><div class="sub">${t.ex}</div></a>`).join('')}</div>
      <p class="faint sm" style="margin-top:14px">Feedback about a specific rule? Use <b>Discuss</b> on that rule's page. It goes to the library that publishes it.</p>`);
  }

  function starSigninModal() {
    openModal(`<div class="modal-h"><h2>Sign in to star rules</h2><button class="x" data-act="close" aria-label="Close">×</button></div>
      <p class="muted">Star this rule if you find it useful.</p>
      <div class="row" style="margin-top:16px"><a class="btn primary" href="#/signin" data-act="close-remember">${icon.gh}Continue with GitHub</a><button class="btn" data-act="close">Not now</button></div>`, true);
  }

  // ---------- Toast + clipboard ----------
  let toastTimer;
  function toast(msg) {
    const root = $('#toast-root');
    root.innerHTML = `<div class="toast" role="status">${esc(msg)}</div>`;
    clearTimeout(toastTimer); toastTimer = setTimeout(() => { root.innerHTML = ''; }, 2200);
  }
  async function copy(text, label) {
    try { await navigator.clipboard.writeText(text); } catch { /* clipboard may be blocked on file:// */ }
    toast(`${label} copied`);
  }

  // ---------- Render ----------
  function applyTheme() {
    if (state.theme === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.dataset.theme = state.theme;
  }
  function render(scroll = true) {
    applyTheme();
    const { parts } = parse();
    const [a, ...rest] = parts;
    let html; let active = '';
    if (a === 'gh') {
      const [b, ...more] = rest;
      html = b === 'authorize' ? ghAuthorize() : b === 'install' ? ghInstall() : b === 'new' ? ghNewIssue() : b === 'issue' ? ghIssue(more) : notFound();
      document.getElementById('app').innerHTML = html;
      if (scroll) window.scrollTo(0, 0);
      return;
    }
    if (!a) html = home();
    else if (a === 'search') { html = search(); active = 'search'; }
    else if (a === 'browse') { const k = rest[0] === 'practices' ? 'practices' : 'techs'; html = browse(k, rest[1] === 'other'); active = k; }
    else if (a === 'libraries') { html = librariesPage(); active = 'libraries'; }
    else if (a === 'g') html = groupPage(rest.join('/'));
    else if (a === 'l') html = libraryPage(rest.join('/'));
    else if (a === 'r') html = rulePage(rest);
    else if (a === 'o') html = ownerPage(rest[0]);
    else if (a === 'signin') html = state.signedIn ? me() : signinPage();
    else if (a === 'me') html = rest[0] === 'private' ? privatePage() : rest[0] === 'add' ? (rest[1] === 'run' ? addRunPage() : addLibraryPage()) : me();
    else html = notFound();
    document.getElementById('app').innerHTML = `${header(active)}<main>${html}</main>${footer()}`;
    if (window.hljs) document.querySelectorAll('.md pre code[class*="language-"]').forEach(el => window.hljs.highlightElement(el));
    if (scroll) window.scrollTo(0, 0);
  }

  // ---------- Events ----------
  document.addEventListener('click', e => {
    const t = e.target.closest('[data-act],[data-copy],[data-sort],[data-libfilter],[data-selall],[data-clearfilters]');
    if (!t) { if (!e.target.closest('.menu')) $('#menu')?.classList.add('hidden'); return; }
    const act = t.dataset.act;
    const r = currentRule();
    if (t.dataset.sort) return setQuery({ sort: t.dataset.sort === t.dataset.default ? null : t.dataset.sort });
    if (t.hasAttribute('data-clearfilters')) return setQuery({ kind: null, impact: null, fabrica: null, mine: null, libs: null, stars: null, used: null });
    if (t.dataset.libfilter !== undefined) return setQuery({ lib: t.dataset.libfilter || null });
    if (t.dataset.selall !== undefined) {
      const lib = libById(parse().parts.slice(1).join('/'));
      return setQuery({ sel: t.dataset.selall === '1' ? [...new Set(lib.rules.map(x => x.group))].join(',') : null });
    }
    if (t.dataset.copy) {
      const lib = libById(parse().parts.slice(1).join('/'));
      const sel = (parse().q.get('sel') || '').split(',').filter(Boolean);
      const map = {
        'prompt-group': () => copy(groupPrompt(r), 'Prompt'), 'cmd-group': () => copy(importCommand(r.lib, [r.group]), 'Command'),
        'prompt-fork': () => copy(forkPrompt(r), 'Prompt'), 'cmd-fork': () => copy(forkCommand(r), 'Command'),
        'prompt-lib': () => copy(libPrompt(lib, sel), 'Prompt'), 'cmd-lib': () => copy(importCommand(lib, sel), 'Command'),
      };
      return map[t.dataset.copy]();
    }
    switch (act) {
      case 'noop': e.preventDefault(); toast('Would open GitHub (not part of the mock)'); break;
      case 'ghlink': e.preventDefault(); toast(`Opens ${t.getAttribute('href').replace('https://', '')}`); break;
      case 'menu': $('#menu').classList.toggle('hidden'); break;
      case 'signout': state.signedIn = false; save(); go('#/'); render(); toast('Signed out'); break;
      case 'remember': state.returnTo = location.hash || '#/'; save(); break;
      case 'remember-add': state.returnTo = '#/me/add'; save(); break;
      case 'feedback': e.preventDefault(); feedbackModal(); break;
      case 'theme': state.theme = { system: 'light', light: 'dark', dark: 'system' }[state.theme]; save(); render(false); break;
      case 'reset': try { localStorage.removeItem(STORE); } catch { /* ignore */ } state = fresh(); go('#/'); render(); toast('Demo reset'); break;
      case 'add-project': addProjectModal(r); break;
      case 'discuss': discussModal(r); break;
      case 'star':
        if (!state.signedIn) { starSigninModal(); break; }
        if (state.stars[r.key]) delete state.stars[r.key]; else state.stars[r.key] = true;
        save(); render(false); toast(state.stars[r.key] ? 'Starred' : 'Star removed'); break;
      case 'close': closeModal(); break;
      case 'close-nav': closeModal(); break;
      case 'close-remember': state.returnTo = location.hash; save(); closeModal(); break;
      case 'close-scrim': if (e.target === t) closeModal(); break;
      case 'skip-private': toast('Okay. Rulemart will only look at your public repos.'); break;
      case 'disconnect': state.private = false; save(); go('#/me'); toast('Private repo access removed'); break;
      case 'gh-authorize': { state.signedIn = true; const back = state.returnTo && !state.returnTo.includes('signin') ? state.returnTo : '#/me'; state.returnTo = null; save(); go(back); setTimeout(() => toast(`Signed in as @${user.login}`), 50); break; }
      case 'gh-cancel': go('#/'); break;
      case 'gh-install': state.private = true; save(); go('#/me'); setTimeout(() => toast('Rulemart can now see the private repos you selected'), 50); break;
      case 'gh-cancel-install': go('#/me/private'); break;
      case 'gh-back': history.back(); break;
      case 'gh-back-hub': go(t.dataset.href); break;
      default: break;
    }
  });

  document.addEventListener('change', e => {
    const t = e.target;
    if (t.dataset.filter) {
      const { q } = parse();
      const cur = q.get(t.dataset.filter);
      return setQuery({ [t.dataset.filter]: t.checked ? t.dataset.val : (cur === t.dataset.val ? null : cur) });
    }
    if (t.dataset.radio) return setQuery({ [t.dataset.radio]: t.value === '0' ? null : t.value });
    if (t.dataset.multi) {
      const vals = new Set((parse().q.get(t.dataset.multi) || '').split(',').filter(Boolean));
      if (t.checked) vals.add(t.dataset.val); else vals.delete(t.dataset.val);
      return setQuery({ [t.dataset.multi]: [...vals].join(',') || null });
    }
    if (t.dataset.gsel) {
      const sel = [...document.querySelectorAll('[data-gsel]:checked')].map(x => x.dataset.gsel);
      return setQuery({ sel: sel.join(',') || null });
    }
    if (t.hasAttribute('data-empty')) return setQuery({ empty: t.checked ? '1' : null });
  });

  document.addEventListener('submit', e => {
    const f = e.target;
    const kind = f.dataset.form;
    if (!kind) return;
    e.preventDefault();
    const data = new FormData(f);
    if (kind === 'search') go(`#/search?q=${encodeURIComponent(data.get('q') || '')}`);
    if (kind === 'addurl') {
      const url = String(data.get('url') || '').trim();
      const m = url.match(/github\.com\/([\w.-]+)\/([\w.-]+?)(?:\.git)?\/?$/i);
      if (!m) return setQuery({ err: 'Enter a GitHub repository URL, like https://github.com/owner/repo.' });
      const id = `${m[1]}/${m[2]}`;
      if (libById(id)) { go(`#/l/${id}`); return toast(`${id} is already on Rulemart`); }
      if (D.publishable.find(p => p.id === id && p.visibility === 'public')) return go(`#/me/add/run?repo=${encodeURIComponent(id)}`);
      if (D.publishable.find(p => p.id === id)) return setQuery({ err: `${id} is private. Private libraries can't be published on Rulemart.` });
      return setQuery({ err: `No rule-library.yaml at the root of github.com/${id}. Is it a Code Rules library? (In this mock, try github.com/josh-padnick/rules-experimental.)` });
    }
    if (kind === 'ghissue') {
      const repo = String(data.get('repo'));
      const num = nextIssueNum(repo);
      state.issues.push({ repo, num, title: String(data.get('title')).trim(), body: String(data.get('body')), ruleKey: String(data.get('rule')) || null, author: user.login, when: 'just now', state: 'open', kind: 'issue' });
      save();
      go(`#/gh/issue/${repo}/${num}`);
    }
  });

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') { closeModal(); $('#menu')?.classList.add('hidden'); }
    if (e.key === 'Enter' && e.target.id === 'topq') go(`#/search?q=${encodeURIComponent(e.target.value)}`);
    if (e.key === '/' && document.activeElement === document.body) { e.preventDefault(); $('#topq')?.focus(); }
  });

  window.addEventListener('hashchange', () => { closeModal(); render(); });
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => render(false));
  render();
})();
