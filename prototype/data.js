/** @fileoverview Fixture data for the Rulemart click-through mock: owners, libraries, rules, discussion, and the signed-in user. Everything here is invented. */

window.RULEMART_DATA = (() => {
  const groups = {
    'techs/go': { name: 'Go', iconUrl: 'https://go.dev/blog/go-brand/Go-Logo/PNG/Go-Logo_LightBlue.png', description: 'Idiomatic, reliable Go.', whenToRead: 'When writing, reviewing, or testing Go code, including module and build changes.' },
    'techs/typescript': { name: 'TypeScript', icon: 'typescript/typescript-original', description: 'Type-safe TypeScript.', whenToRead: 'When writing or reviewing TypeScript, including type definitions.' },
    'techs/react': { name: 'React', icon: 'react/react-original', description: 'Components, hooks, and state.', whenToRead: 'When writing or reviewing React components and hooks.' },
    'techs/playwright': { name: 'Playwright', icon: 'playwright/playwright-original', description: 'Reliable browser tests.', whenToRead: 'When writing or debugging Playwright tests.' },
    'techs/python': { name: 'Python', icon: 'python/python-original', description: 'Readable, typed Python.', whenToRead: 'When writing or reviewing Python code.' },
    'techs/rust': { name: 'Rust', icon: 'rust/rust-original', ink: true, description: 'Safe, idiomatic Rust.', whenToRead: 'When writing or reviewing Rust code.' },
    'techs/nextjs': { name: 'Next.js', icon: 'nextjs/nextjs-original', ink: true, description: 'App Router and server components.', whenToRead: 'When building pages, routes, or data fetching in Next.js.' },
    'techs/postgresql': { name: 'PostgreSQL', icon: 'postgresql/postgresql-original', description: 'Schemas, queries, and migrations.', whenToRead: 'When writing SQL or database migrations for PostgreSQL.' },
    'techs/docker': { name: 'Docker', icon: 'docker/docker-original', description: 'Small, reproducible images.', whenToRead: 'When writing or changing Dockerfiles.' },
    'techs/kubernetes': { name: 'Kubernetes', icon: 'kubernetes/kubernetes-original', description: 'Workloads that behave under load.', whenToRead: 'When writing or changing Kubernetes manifests.' },
    'techs/terraform': { name: 'Terraform', icon: 'terraform/terraform-original', description: 'Safe infrastructure changes.', whenToRead: 'When writing or changing Terraform configuration.' },
    'practices/testing': { lucide: 'flask-conical', name: 'Testing', description: 'Verify behavior with meaningful tests.', whenToRead: 'When adding or changing behavior, fixing a bug, or modifying tests, even when no test files are in the diff.' },
    'practices/comments': { lucide: 'message-square-text', name: 'Comments', description: 'Comments that stay true.', whenToRead: 'When writing or reviewing comments, file headers, or doc comments.' },
    'practices/error-handling': { lucide: 'triangle-alert', name: 'Error handling', description: 'Errors people can act on.', whenToRead: 'When creating, wrapping, returning, or showing errors.' },
    'practices/observability': { lucide: 'activity', name: 'Observability', description: 'Logs, metrics, and traces.', whenToRead: 'When adding or changing logs, metrics, or traces.' },
    'practices/resilience': { lucide: 'life-buoy', name: 'Resilience', description: 'Timeouts, retries, and failure handling.', whenToRead: 'When calling networks or other services, or adding retries.' },
    'practices/documentation': { lucide: 'book-open', name: 'Documentation', description: 'Docs that match the code.', whenToRead: 'When writing or changing READMEs, guides, or docstrings.' },
    'practices/code-review': { lucide: 'git-pull-request', name: 'Code review', description: 'How to review agent-written changes.', whenToRead: 'When reviewing a pull request or a proposed plan.' },
    // Non-canonical: names individual libraries chose for themselves.
    'techs/golang': { name: 'Golang', canonical: false, similarTo: 'techs/go', description: 'Go rules.', whenToRead: 'When writing Go.' },
    'practices/payments-compliance': { name: 'Payments compliance', canonical: false, description: 'PCI and card-data handling.', whenToRead: 'When code touches card numbers or payment tokens.' },
    'practices/agent-hygiene': { name: 'Agent hygiene', canonical: false, similarTo: 'practices/code-review', description: 'Keeping agent output reviewable.', whenToRead: 'When an agent opens or updates a pull request.' },
  };
  // Every group without canonical: false is on the canonical list that Rulemart groups across libraries.
  Object.values(groups).forEach(g => { if (g.canonical !== false) g.canonical = true; });

  const owners = {
    fabricahq: { type: 'org', name: 'Fabrica', verified: 'fabricahq.com', initials: 'F', real: true, bio: 'Tools for teams building software with AI agents.' },
    acme: { type: 'org', name: 'Acme Corp', verified: 'acme.dev', initials: 'A', bio: 'Payments infrastructure.' },
    gopherworks: { type: 'user', name: 'Sam Rivera', verified: null, initials: 'SR', bio: 'Go since 1.0.' },
    northwind: { type: 'org', name: 'Northwind Labs', verified: null, initials: 'N', bio: 'Logistics software.' },
    'josh-padnick': { type: 'user', name: 'Josh Padnick', verified: null, initials: 'JP', real: true, bio: 'Building Fabrica.' },
  };

  const user = { login: 'josh-padnick', name: 'Josh Padnick', initials: 'JP', orgs: ['fabricahq'] };

  const c = s => s.trim();

  const featuredBody = c(`
<p>A comment is an index entry, not a narration of the next line. A file comment lets a reader decide whether to open the file. An export comment tells a caller what they get back, including rules the signature does not show. A body comment records a constraint that names and types cannot express.</p>
<p>Agent-authored code fails this rule by omission more than by narration: files land with no header, exports carry <code>@param</code> tags that repeat the signature, and a magic sleep ships with no reason. When the reason is missing, do not invent one; a guessed rationale becomes a spec for the next agent.</p>
<h3>File role and exported contract</h3>
<p><b>Incorrect:</b> no file header, and the export comment repeats the name and signature.</p>
<pre><code>/**
 * Parses transactions.
 * @param csv - The CSV string to parse
 * @returns The parsed transactions
 */
export function parseTransactions(csv: string): Transaction[]</code></pre>
<p><b>Correct:</b> the file states its role, and the export describes the result and supported input.</p>
<pre><code>/**
 * @fileoverview Ingests the ledger's id,posted_at,amount CSV export.
 * Not a general CSV parser: fields are never quoted.
 */

/**
 * Parse the ledger export into transactions with amounts in integer cents.
 * Throws on a row with the wrong field count or an unparsable amount.
 */
export function parseTransactions(csv: string): Transaction[]</code></pre>
<h3>Hidden constraints</h3>
<p>Record the constraint the code cannot show, such as a vendor rate limit, next to the value it explains.</p>
<pre><code>// The vendor rejects more than 4 calls per second per API key.
const VENDOR_MIN_CALL_INTERVAL_MS = 250;</code></pre>
<h3>Validation</h3>
<p>Every new file has a header that states its role. Every export comment describes the result or a rule the signature hides. No comment restates the next line.</p>`);

  const simple = (lead, bad, good, check) => c(`
<p>${lead}</p>
<p><b>Incorrect:</b></p><pre><code>${bad}</code></pre>
<p><b>Correct:</b></p><pre><code>${good}</code></pre>
<h3>Validation</h3><p>${check}</p>`);

  // A rule with supporting assets. Links are relative Markdown links, as a library author writes them:
  // the rule's own files are in assets/<rule>/ beside it, and ../../assets/ reaches the library's shared files.
  const changedBehaviorBody = c(`
<p>Every behavior change comes with a test that would fail without it. This includes bug fixes, even when the diff touches no test files.</p>
<p><img src="assets/test-changed-behavior/red-green-loop.svg" alt="Write a test that fails, change the code until it passes, then revert the change to confirm the test fails again."></p>
<p><b>Incorrect:</b></p><pre><code>// Fixed the rounding bug. No test.</code></pre>
<p><b>Correct:</b></p><pre><code>it("rounds half up to the nearest cent", () => {
  expect(round(1.005)).toBe(1.01);
});</code></pre>
<h3>Validation</h3><p>Reverting the change makes at least one test fail. See <a href="assets/test-changed-behavior/why-revert-check.md">why the revert check works</a> and the <a href="assets/test-changed-behavior/rounding-cases.json">rounding cases</a> behind this example. Terms such as <i>regression test</i> are defined in the <a href="../../assets/testing-glossary.md">testing glossary</a>.</p>`);
  // Paths are relative to the rule's asset directory, practices/testing/assets/test-changed-behavior/.
  // flaky-test-checklist.md is not linked from the rule, but Code Rules still copies it with the rule.
  const changedBehaviorAssets = [
    { path: 'red-green-loop.svg', type: 'image', size: '2.4 KB', alt: 'Write a test that fails, change the code until it passes, then revert the change to confirm the test fails again.' },
    { path: 'why-revert-check.md', type: 'markdown', size: '1.1 KB', html: c(`
<h2>Why the revert check works</h2>
<p>A test written after a fix often passes both with and without the fix. It runs the code but does not pin the behavior that changed, so a later refactor can quietly undo the fix.</p>
<p>Reverting the change is the cheapest way to prove the test does its job:</p>
<ol><li>Revert the behavior change, but keep the new test.</li><li>Run the test. It should fail, and the failure should describe the bug.</li><li>Restore the change. The test passes.</li></ol>
<p>If the test still passes in step 2, it is testing something else. Tighten the assertion until it fails for the right reason.</p>
<h3>When a full revert is impractical</h3>
<p>Revert only the line that decides the behavior, or temporarily hard-code the old result. The goal is the same: watch the test fail once.</p>
<p>See <i>regression test</i> in the <a href="../../../../assets/testing-glossary.md">testing glossary</a>.</p>`) },
    { path: 'rounding-cases.json', type: 'json', size: '312 B', text: `{
  "description": "Inputs that the old rounding code got wrong.",
  "cases": [
    { "input": 1.005, "expected": 1.01 },
    { "input": 2.675, "expected": 2.68 },
    { "input": -1.005, "expected": -1.01 },
    { "input": 0.125, "expected": 0.13 }
  ]
}` },
    { path: 'flaky-test-checklist.md', type: 'markdown', size: '640 B', html: c(`
<h2>When the new test is flaky</h2>
<p>A regression test that fails only sometimes is worse than none. Before merging, check:</p>
<ul><li>The test does not depend on the current time, time zone, or locale.</li><li>It does not share state with other tests or rely on test order.</li><li>Every wait has an explicit condition, not a fixed sleep.</li></ul>`) },
  ];
  // Files in the library's root assets/ directory. Code Rules copies the whole directory into a project when a selected rule links into it.
  const fabSharedAssets = [
    { path: 'testing-glossary.md', type: 'markdown', size: '1.6 KB', html: c(`
<h2>Testing glossary</h2>
<h3>Regression test</h3><p>A test added with a bug fix that fails on the old code and passes on the new code, so the bug cannot return unnoticed.</p>
<h3>Characterization test</h3><p>A test that records what existing code does today, written before changing code that has no tests.</p>
<h3>Test double</h3><p>A stand-in for a real dependency in a test, such as a fake, stub, or mock.</p>`) },
    { path: 'test-naming.md', type: 'markdown', size: '720 B', html: c(`
<h2>Naming tests</h2>
<p>Name a test after the behavior it proves, not the function it calls: <code>rounds half up to the nearest cent</code>, not <code>test_round</code>.</p>`) },
  ];

  // Library releases: each is one release/<n> tag whose message holds the release record. Rule histories below
  // name the library release that added or changed them; Rulemart would build both from the release records.
  const fabReleases = [
    { n: 1, date: '7 months ago' },
    { n: 2, date: '4 months ago' },
    { n: 3, date: '3 months ago', libraryFiles: ['techs/playwright/_group.yaml', 'techs/react/_group.yaml'] },
    { n: 4, date: '5 weeks ago' },
    { n: 5, date: '3 days ago', libraryFiles: ['assets/testing-glossary.md'] },
  ];

  const libraries = [
    {
      id: 'fabricahq/public-rules', owner: 'fabricahq', name: 'Fabrica Public Rules', description: 'Rules for teams building software with AI agents.',
      license: 'MIT', releases: fabReleases, addedBy: 'josh-padnick', addedOn: '2 Mar 2026', fabrica: true, usedBy: 1516, sharedAssets: fabSharedAssets,
      rules: [
        { slug: 'wrap-errors-with-operation', group: 'techs/go', title: 'Wrap errors with the operation that failed', impact: 'HIGH', tags: ['errors'], usedBy: 1102, net30: 74, stars: 340,
          whenToRead: 'When returning an error from a Go function that called something that can fail.',
          body: simple('Wrap every returned error with the operation that failed, using <code>%w</code> so callers can still inspect it.', 'if err != nil {\n    return err\n}', 'if err != nil {\n    return fmt.Errorf("load config %s: %w", path, err)\n}', 'Every <code>return err</code> after a failing call adds context, and tests can still match the cause with <code>errors.Is</code>.'),
          added: 1, changes: [{ release: 4, change: 'patch', summary: 'Clarify when not to wrap sentinel errors.' }] },
        { slug: 'avoid-package-level-state', group: 'techs/go', title: 'Avoid package-level mutable state', impact: 'MEDIUM', tags: ['concurrency'], usedBy: 688, net30: 21, stars: 97,
          whenToRead: 'When adding variables at package scope or singletons in Go.',
          body: simple('Pass dependencies explicitly instead of storing them in package-level variables.', 'var db *sql.DB\n\nfunc Save(u User) error { return insert(db, u) }', 'type Store struct{ db *sql.DB }\n\nfunc (s *Store) Save(u User) error { return insert(s.db, u) }', 'No package-level <code>var</code> holds a connection, client, or cache that tests would need to reset.'),
          added: 1, changes: [] },
        { slug: 'close-what-you-open', group: 'techs/go', title: 'Close what you open, right after checking the error', impact: 'HIGH', tags: ['resources'], usedBy: 954, net30: 33, stars: 121,
          whenToRead: 'When opening files, response bodies, rows, or other closable resources in Go.',
          body: simple('Defer <code>Close</code> immediately after the error check that follows the open.', 'resp, err := http.Get(url)\nbody, _ := io.ReadAll(resp.Body)', 'resp, err := http.Get(url)\nif err != nil {\n    return err\n}\ndefer resp.Body.Close()', 'Every opened resource has a <code>defer Close</code> on the line after its error check.'),
          added: 2, addedSummary: 'Add a rule about closing resources in Go.', changes: [] },
        { slug: 'narrow-unknown-values', group: 'techs/typescript', title: 'Narrow unknown values before use', impact: 'HIGH', tags: ['types', 'validation'], usedBy: 1210, net30: 88, stars: 286,
          whenToRead: 'When handling parsed JSON, API responses, or other data typed as unknown or any.',
          body: simple('Treat external data as <code>unknown</code> and narrow it with a schema or type guard before reading fields.', 'const user = JSON.parse(text) as User;\nsendEmail(user.email);', 'const user = UserSchema.parse(JSON.parse(text));\nsendEmail(user.email);', 'No <code>as</code> cast turns parsed or fetched data into a domain type without validation.'),
          added: 1, changes: [{ release: 5, change: 'minor', summary: 'Add a note on validating at the boundary only once.' }] },
        { slug: 'model-valid-states', group: 'techs/typescript', title: 'Model only valid states', impact: 'MEDIUM', tags: ['types'], usedBy: 802, net30: 40, stars: 154,
          whenToRead: 'When defining types for data with modes, statuses, or optional fields that depend on each other.',
          body: simple('Use discriminated unions so impossible combinations cannot be represented.', 'type Load = { loading: boolean; data?: Data; error?: Error };', "type Load =\n  | { status: 'loading' }\n  | { status: 'done'; data: Data }\n  | { status: 'failed'; error: Error };", 'Types with a status field use a union where each status carries only its own fields.'),
          added: 1, changes: [] },
        { slug: 'locate-by-role', group: 'techs/playwright', title: 'Locate elements by role, not CSS selectors', impact: 'HIGH', tags: ['testing', 'accessibility'], usedBy: 732, net30: 64, stars: 188,
          whenToRead: 'When selecting elements in a Playwright test.',
          body: simple('Use role and accessible-name locators so tests survive markup changes and check accessibility too.', "page.locator('.btn-primary.submit')", "page.getByRole('button', { name: 'Submit order' })", 'No test selects by class name or nth-child.'),
          added: 3, addedSummary: 'Add the Playwright group.', changes: [] },
        { slug: 'web-first-assertions', group: 'techs/playwright', title: 'Wait with web-first assertions, not timeouts', impact: 'MEDIUM', tags: ['flakiness'], usedBy: 690, net30: 41, stars: 142,
          whenToRead: 'When a Playwright test waits for the page to change.',
          body: simple('Use assertions that retry until the condition holds instead of fixed waits.', 'await page.waitForTimeout(2000);\nexpect(await page.textContent(".status")).toBe("Paid");', "await expect(page.getByText('Paid')).toBeVisible();", 'No test calls waitForTimeout.'),
          added: 3, addedSummary: 'Add the Playwright group.', changes: [] },
        { slug: 'type-public-functions', group: 'techs/python', title: 'Type-annotate every public function', impact: 'MEDIUM', tags: ['types'], usedBy: 512, net30: 38, stars: 97,
          whenToRead: 'When adding or changing public Python functions or methods.',
          body: simple('Annotate parameters and return types on public functions so type checkers and agents can rely on them.', 'def load(path, strict=False):', 'def load(path: Path, strict: bool = False) -> Config:', 'mypy or pyright passes in strict mode on changed modules.'),
          added: 4, addedSummary: 'Add a rule about typing public Python functions.', changes: [] },
        { slug: 'fetch-in-server-components', group: 'techs/nextjs', title: 'Fetch data in Server Components', impact: 'MEDIUM', tags: ['data-fetching'], usedBy: 455, net30: 57, stars: 119,
          whenToRead: 'When loading data for a Next.js App Router page.',
          body: simple('Fetch on the server by default; reach for client fetching only for interactive, user-specific data.', "'use client';\nuseEffect(() => { fetch('/api/posts').then(...) }, []);", 'export default async function Page() {\n  const posts = await getPosts();\n  return &lt;PostList posts={posts} /&gt;;\n}', 'Pages do not fetch initial data in useEffect.'),
          added: 5, addedSummary: 'Add a rule about loading page data on the server.', changes: [] },
        { slug: 'create-indexes-concurrently', group: 'techs/postgresql', title: 'Create indexes concurrently in migrations', impact: 'HIGH', tags: ['migrations'], usedBy: 621, net30: 29, stars: 156,
          whenToRead: 'When a migration adds an index to an existing PostgreSQL table.',
          body: simple('Use CREATE INDEX CONCURRENTLY so the migration does not lock writes on a live table.', 'CREATE INDEX idx_orders_user ON orders (user_id);', 'CREATE INDEX CONCURRENTLY idx_orders_user ON orders (user_id);', 'Index migrations on existing tables use CONCURRENTLY and run outside a transaction.'),
          added: 4, addedSummary: 'Add a rule about index migrations on live tables.', changes: [] },
        { slug: 'keep-state-local', group: 'techs/react', title: 'Keep state as local as possible', impact: 'MEDIUM', tags: ['state'], usedBy: 640, net30: 52, stars: 133,
          whenToRead: 'When adding state to React components or context providers.',
          body: simple('Put state in the lowest component that needs it. Lift it only when a sibling needs it too.', 'const [open, setOpen] = useAppStore(s => [s.menuOpen, s.setMenuOpen]);', 'const [open, setOpen] = useState(false);', 'Global stores and context hold only state that several distant components read.'),
          added: 3, addedSummary: 'Add the React group.', changes: [] },
        { slug: 'name-interactive-controls', group: 'techs/react', title: 'Give every interactive control an accessible name', impact: 'HIGH', tags: ['accessibility'], usedBy: 598, net30: 47, stars: 162,
          whenToRead: 'When adding buttons, links, inputs, or custom controls in React.',
          body: simple('Every control needs a name a screen reader can announce: visible text, a label, or <code>aria-label</code>.', '&lt;button onClick={close}&gt;&lt;XIcon /&gt;&lt;/button&gt;', '&lt;button onClick={close} aria-label="Close dialog"&gt;&lt;XIcon /&gt;&lt;/button&gt;', 'Querying each control by role and name in tests finds it.'),
          added: 3, addedSummary: 'Add the React group.', changes: [] },
        { slug: 'verify-retry-limits', group: 'practices/testing', title: 'Verify retry limits in tests', impact: 'HIGH', tags: ['retries'], usedBy: 0, net30: 0, stars: 0,
          whenToRead: 'Before adding or changing bounded retries, test that requests stop at the configured limit.',
          body: simple('When code retries, write a test that proves it stops at the configured limit.', 'it("retries", async () => {\n  await callWithRetry(flaky);\n});', 'it("stops after 3 attempts", async () => {\n  await expect(callWithRetry(alwaysFails)).rejects.toThrow();\n  expect(alwaysFails).toHaveBeenCalledTimes(3);\n});', 'The test fails if one extra retry is added.'),
          added: 5, changes: [] },
        { slug: 'test-failure-paths', group: 'practices/testing', title: 'Test failure paths, not just success', impact: 'HIGH', tags: ['errors'], usedBy: 1190, net30: 60, stars: 201,
          whenToRead: 'When adding or changing code that can fail, reject input, or time out.',
          body: simple('For every failure the code handles, add a test that triggers it and checks the result.', 'it("saves a user", ...)', 'it("saves a user", ...)\nit("rejects a duplicate email", ...)\nit("surfaces a database timeout", ...)', 'Each handled error branch is reached by at least one test.'),
          added: 1, changes: [{ release: 2, change: 'minor', summary: 'Add timeouts as a failure path.' }] },
        { slug: 'test-changed-behavior', group: 'practices/testing', title: 'Test the behavior you changed', impact: 'MEDIUM-HIGH', tags: [], usedBy: 1320, net30: 95, stars: 244,
          whenToRead: 'When changing behavior or fixing a bug, even when no test files are in the diff.',
          body: changedBehaviorBody, assets: changedBehaviorAssets,
          added: 1, changes: [] },
        { slug: 'comment-role-result-and-constraints', group: 'practices/comments', title: 'Comment the role, the result, and the hidden constraint', impact: 'MEDIUM', tags: ['typescript', 'comments', 'documentation'], usedBy: 947, net30: 86, stars: 214,
          whenToRead: 'Before writing or reviewing comments, file headers, or doc comments on exported code.',
          body: featuredBody, featured: true,
          added: 1, changes: [
            { release: 2, change: 'minor', summary: 'Add the hidden-constraint example.' },
            { release: 3, change: 'major', summary: 'Require a file header on every new file, not only on exported modules.' },
            { release: 4, change: 'patch', summary: 'Clarify when a private helper needs a comment. Fixes #198.' },
          ],
          discussion: [
            { kind: 'pr', num: 226, title: 'Exempt generated files from file headers', author: 'josh-padnick', comments: 4, state: 'open', relation: 'changes', note: 'linked to #221', when: '2 days ago' },
            { kind: 'issue', num: 221, title: 'Headers on generated code', author: 'josh-padnick', comments: 8, reactions: 12, state: 'open', when: '6 days ago', last: 'PR #226 has proposed wording' },
            { kind: 'issue', num: 219, title: 'Stopped our agents from inventing rationales', author: 'dkato', comments: 2, reactions: 7, state: 'open', when: '1 week ago' },
            { kind: 'issue', num: 214, title: 'Does this apply to Go?', author: 'priya-r', comments: 3, reactions: 9, state: 'open', when: '2 weeks ago' },
            { kind: 'pr', num: 203, title: 'Link the comments rule from the Go group README', author: 'mlopez', comments: 1, state: 'open', relation: 'mentions', when: '3 weeks ago' },
            { kind: 'issue', num: 198, title: 'When does a private helper need a comment?', author: 'mlopez', comments: 11, state: 'closed', closedIn: '2.0.1', when: '2 months ago' },
          ],
          removals: [
            { kind: 'Excluded', project: 'lumen-labs/api', at: '2.0.0', when: '12 Aug', reason: 'Agents over-comment when given this; we rely on names and tests.' },
            { kind: 'Replaced', project: 'openledger/sdk-ts', at: '2.0.1', when: '20 Sep', reason: 'We want stricter: no body comments without a linked issue.', with: 'local/practices/comments/link-every-workaround' },
            { kind: 'Group removed', project: 'tidepool/web', at: '1.1.0', when: '3 Jul', reason: null },
          ] },
        { slug: 'dont-invent-rationale', group: 'practices/comments', title: "Don't invent a rationale in comments", impact: 'MEDIUM', tags: ['comments'], usedBy: 903, net30: 70, stars: 176,
          whenToRead: 'When a comment explains why code does something.',
          body: simple('If you do not know why the code works this way, say so or leave the reason out. Never guess.', '// Sleep for performance reasons.\nawait sleep(250);', '// TODO(#412): the reason for this delay is unknown; do not remove without testing the vendor sync.\nawait sleep(250);', 'Every "why" comment is backed by a link, a test, or a known constraint.'),
          added: 2, addedSummary: 'Add a rule against guessed rationales in comments.', changes: [] },
        { slug: 'make-errors-actionable', lang: 'plaintext', group: 'practices/error-handling', title: 'Make error messages actionable', impact: 'MEDIUM', tags: ['ux'], usedBy: 1044, net30: 58, stars: 190,
          whenToRead: 'When writing or reviewing validation errors shown to users.',
          body: simple('Error messages must explain what went wrong and how to fix it.', '"Upload failed."', '"File too large. Choose a file up to 10 MB."', 'Upload an oversized file and check that the error states the size limit and how to proceed.'),
          added: 1, changes: [] },
        { slug: 'preserve-error-context', group: 'practices/error-handling', title: 'Preserve error context across boundaries', impact: 'HIGH', tags: [], usedBy: 986, net30: 41, stars: 139,
          whenToRead: 'When catching an error and rethrowing, returning, or converting it.',
          body: simple('Keep the original error as the cause when you wrap or convert it.', 'catch (e) {\n  throw new Error("save failed");\n}', 'catch (e) {\n  throw new Error("save failed", { cause: e });\n}', 'Logs for a failed request show the root cause, not only the outer message.'),
          added: 1, changes: [] },
        { slug: 'never-swallow-errors', group: 'practices/error-handling', title: 'Never swallow errors silently', impact: 'HIGH', tags: [], usedBy: 1150, net30: 49, stars: 173,
          whenToRead: 'When writing catch blocks, error callbacks, or ignored return values.',
          body: simple('Handle, return, or log every error. An empty catch block hides failures.', 'try { await sync(); } catch {}', "try { await sync(); } catch (e) { log.warn('sync failed; will retry', { err: e }); }", 'No empty catch blocks or discarded error returns.'),
          added: 1, changes: [] },
        { slug: 'no-secrets-in-logs', group: 'practices/observability', title: 'Never write secrets to logs', impact: 'CRITICAL', tags: ['security', 'logging'], usedBy: 1402, net30: 102, stars: 309,
          whenToRead: 'When adding or changing log statements, error messages, or telemetry.',
          body: simple('Never log passwords, tokens, keys, or full request bodies that may contain them.', 'log.info("login", { email, password });', 'log.info("login", { email });', 'Search new log calls for credential fields; none may appear.'),
          added: 1, changes: [] },
        { slug: 'log-structured-fields', group: 'practices/observability', title: 'Log with structured fields', impact: 'MEDIUM', tags: ['logging'], usedBy: 504, net30: 12, stars: 88,
          whenToRead: 'When adding log statements.',
          body: simple('Put variable data in fields, not in the message string.', 'log.info(`user ${id} paid ${amount}`);', 'log.info("payment received", { userId: id, amountCents: amount });', 'Log messages are constant strings; data lives in fields.'),
          added: 5, addedSummary: 'Add a rule about keeping log data in fields.', changes: [] },
      ],
      insights: [
        { rule: 'comment-role-result-and-constraints', using: 947, ever: 1061, removed: 114, reason: '"Generated code gets headers that codegen wipes." (17 similar, 41 with no reason)' },
        { rule: 'log-structured-fields', using: 504, ever: 526, removed: 22, reason: 'Replaced by acme › use-otel-attributes (9 projects)' },
        { rule: 'avoid-package-level-state', using: 688, ever: 731, removed: 43, reason: '"We use a DI container; this conflicts." (11 similar)' },
      ],
    },
    {
      id: 'acme/.code-rules', owner: 'acme', name: 'Acme Engineering Defaults', description: "Shared defaults for Acme's payment services.",
      license: 'Apache-2.0', releases: [{ n: 1, date: '5 months ago' }, { n: 2, date: '6 weeks ago' }],
      addedBy: 'acme-bot', addedOn: '14 Apr 2026', usedBy: 402,
      rules: [
        { slug: 'avoid-effect-for-derived-state', group: 'techs/react', title: "Don't use effects for derived state", impact: 'MEDIUM', tags: ['hooks'], usedBy: 311, net30: 19, stars: 64,
          whenToRead: 'When a React effect sets state computed from props or other state.',
          body: simple('Compute derived values during render instead of syncing them with an effect.', 'useEffect(() => setTotal(sum(items)), [items]);', 'const total = sum(items);', 'No effect only calls a state setter with values derived from props or state.'),
          added: 1, changes: [] },
        { slug: 'retry-only-idempotent', group: 'practices/resilience', title: 'Retry only idempotent requests', impact: 'CRITICAL', tags: ['retries'], usedBy: 402, net30: 30, stars: 57,
          whenToRead: 'Before adding retries to a network call, confirm the operation is safe to repeat.',
          body: simple('Retry only operations that are safe to repeat, or make them safe with an idempotency key.', 'await retry(() => charge(card, amount));', 'await retry(() => charge(card, amount, { idempotencyKey }));', 'Every retried write carries an idempotency key.'),
          added: 2, addedSummary: 'Add resilience, container, and infrastructure rules.', changes: [] },
        { slug: 'set-timeouts-on-network-calls', group: 'practices/resilience', title: 'Set a timeout on every network call', impact: 'HIGH', tags: ['timeouts'], usedBy: 377, net30: 22, stars: 48,
          whenToRead: 'When making HTTP, RPC, or database calls.',
          body: simple('Every outbound call has an explicit timeout.', 'await fetch(url);', 'await fetch(url, { signal: AbortSignal.timeout(5_000) });', 'No client is constructed without a timeout.'),
          added: 2, addedSummary: 'Add resilience, container, and infrastructure rules.', changes: [] },
        { slug: 'pin-base-images', group: 'techs/docker', title: 'Pin base images by digest', impact: 'HIGH', tags: ['supply-chain'], usedBy: 214, net30: 12, stars: 44,
          whenToRead: 'When writing or changing a Dockerfile FROM line.',
          body: simple('Pin base images to a digest so builds are reproducible and cannot change underneath you.', 'FROM node:20', 'FROM node:20@sha256:4b1d...e9f2', 'Every FROM line includes a digest.'),
          added: 2, addedSummary: 'Add resilience, container, and infrastructure rules.', changes: [] },
        { slug: 'set-resource-requests', group: 'techs/kubernetes', title: 'Set resource requests and limits on every container', impact: 'HIGH', tags: ['reliability'], usedBy: 198, net30: 9, stars: 39,
          whenToRead: 'When writing or changing a Kubernetes Deployment or Pod spec.',
          body: simple('Give every container CPU and memory requests, and a memory limit, so the scheduler can place it and noisy neighbors cannot starve it.', 'containers:\n  - name: api\n    image: acme/api', 'containers:\n  - name: api\n    image: acme/api\n    resources:\n      requests: { cpu: 250m, memory: 256Mi }\n      limits: { memory: 512Mi }', 'No container spec lacks resources.requests.'),
          added: 2, addedSummary: 'Add resilience, container, and infrastructure rules.', changes: [] },
        { slug: 'no-secrets-in-terraform', group: 'techs/terraform', title: 'Never hardcode secrets in Terraform', impact: 'CRITICAL', tags: ['security'], usedBy: 176, net30: 7, stars: 52,
          whenToRead: 'When a Terraform resource needs a password, key, or token.',
          body: simple('Read secrets from a secret manager or variables marked sensitive. Never write them into .tf files.', 'password = "hunter2"', 'password = data.aws_secretsmanager_secret_version.db.secret_string', 'No string literal in .tf files looks like a credential.'),
          added: 2, addedSummary: 'Add resilience, container, and infrastructure rules.', changes: [] },
        { slug: 'never-log-card-numbers', group: 'practices/payments-compliance', title: 'Never log or persist full card numbers', impact: 'CRITICAL', tags: ['pci'], usedBy: 41, net30: 2, stars: 11,
          whenToRead: 'When code handles card numbers, CVVs, or payment tokens.',
          body: simple('Store and log only tokens or the last four digits. Full card numbers never leave the vault.', 'log.info("charge", { card: req.cardNumber });', 'log.info("charge", { cardLast4: token.last4 });', 'No log line, database column, or error message contains a full PAN.'),
          added: 2, addedSummary: 'Add resilience, container, and infrastructure rules.', changes: [] },
        { slug: 'no-sleeps-in-tests', group: 'practices/testing', title: "Don't sleep in tests; wait for conditions", impact: 'MEDIUM', tags: ['flakiness'], usedBy: 289, net30: 9, stars: 71,
          whenToRead: 'When a test waits for something asynchronous.',
          body: simple('Wait for the condition you need, not a fixed amount of time.', 'await sleep(500);\nexpect(queue.size).toBe(0);', 'await waitFor(() => expect(queue.size).toBe(0));', 'No test calls sleep or setTimeout to wait for work to finish.'),
          added: 1, changes: [] },
      ],
    },
    {
      id: 'gopherworks/go-rules', owner: 'gopherworks', name: 'Gopherworks Go Rules', description: 'Go rules from years of production services.',
      license: 'MIT', releases: [{ n: 1, date: '8 months ago' }, { n: 2, date: '2 months ago' }],
      addedBy: 'gopherworks', addedOn: '20 Feb 2026', usedBy: 188,
      rules: [
        { slug: 'backoff-with-jitter', group: 'techs/go', title: 'Back off with jitter between attempts', impact: 'MEDIUM', tags: ['retries'], usedBy: 188, net30: 6, stars: 9,
          whenToRead: 'Before writing a retry loop in Go, use capped exponential backoff with jitter.',
          body: simple('Use capped exponential backoff with random jitter between retries.', 'for i := 0; i < 5; i++ {\n    time.Sleep(time.Second)\n}', 'b := backoff.NewExponential(100*time.Millisecond, 5*time.Second)\nfor attempt := range b.Attempts(5) { ... }', 'Retry loops never sleep a fixed interval.'),
          added: 1, changes: [{ release: 2, change: 'patch', summary: 'Clarify that the backoff cap applies to each attempt.' }] },
        { slug: 'accept-interfaces-return-structs', group: 'techs/go', title: 'Accept interfaces, return structs', impact: 'MEDIUM', tags: ['api-design'], usedBy: 377, net30: 11, stars: 96,
          whenToRead: 'When designing Go function signatures and constructors.',
          body: simple('Take the narrowest interface you need; return concrete types.', 'func NewService(db *sql.DB) ServiceInterface', 'func NewService(q Querier) *Service', 'Constructors return concrete types.'),
          added: 1, changes: [] },
        { slug: 'table-driven-tests', group: 'techs/go', title: 'Prefer table-driven tests', impact: 'LOW-MEDIUM', tags: ['testing'], usedBy: 164, net30: 4, stars: 31,
          whenToRead: 'When writing several Go tests that differ only by input and expected output.',
          body: simple('Use a table of cases with <code>t.Run</code> when tests share setup and differ by data.', 'func TestAdd1(t *testing.T) {...}\nfunc TestAdd2(t *testing.T) {...}', 'for _, tc := range cases {\n    t.Run(tc.name, func(t *testing.T) {...})\n}', 'Near-identical test functions are merged into a table.'),
          added: 1, changes: [] },
      ],
    },
    {
      id: 'northwind/engineering-rules', owner: 'northwind', name: 'Northwind Engineering Rules', description: "Rules for Northwind's backend services.",
      license: 'CC-BY-4.0', releases: [{ n: 1, date: '3 months ago' }],
      addedBy: 'kwame-n', addedOn: '1 Jul 2026', usedBy: 64,
      rules: [
        { slug: 'pass-context-first', group: 'techs/golang', title: 'Pass context as the first parameter', impact: 'HIGH', tags: ['context'], usedBy: 290, net30: 15, stars: 88,
          whenToRead: 'When writing Go functions that do I/O or may be cancelled.',
          body: simple('Accept <code>context.Context</code> as the first parameter and pass it through.', 'func Fetch(url string) ([]byte, error)', 'func Fetch(ctx context.Context, url string) ([]byte, error)', 'Functions that do I/O take a context first.'),
          added: 1, changes: [] },
        { slug: 'return-result-dont-panic', group: 'techs/rust', title: 'Return Result instead of panicking in libraries', impact: 'HIGH', tags: ['errors'], usedBy: 58, net30: 4, stars: 21,
          whenToRead: 'When a Rust library function can fail.',
          body: simple('Library code returns Result and lets the caller decide. Reserve panics for broken invariants.', 'let cfg = fs::read_to_string(path).unwrap();', 'let cfg = fs::read_to_string(path)?;', 'No unwrap or expect on fallible I/O in library crates.'),
          added: 1, changes: [] },
        { slug: 'trace-every-request', group: 'practices/observability', title: 'Propagate trace context on every request', impact: 'MEDIUM', tags: ['tracing'], usedBy: 51, net30: 3, stars: 14,
          whenToRead: 'When making outbound requests from a service.',
          body: simple('Forward trace headers on every outbound call.', 'http.Get(url)', 'req = req.WithContext(ctx)\notel.Inject(ctx, req.Header)', 'Traces show a single span tree across services.'),
          added: 1, changes: [] },
        { slug: 'one-behavior-per-test', group: 'practices/testing', title: 'Keep each test focused on one behavior', impact: 'LOW-MEDIUM', tags: [], usedBy: 64, net30: 2, stars: 12,
          whenToRead: 'When writing or splitting tests.',
          body: simple('Each test checks one behavior, named after it.', 'it("works", () => { /* 12 assertions */ })', 'it("rejects an expired token", ...)\nit("refreshes a token near expiry", ...)', 'A failing test name says what broke.'),
          added: 1, changes: [] },
      ],
    },
    {
      id: 'josh-padnick/.code-rules', owner: 'josh-padnick', name: "Josh's Personal Rules", description: 'Rules I use across side projects.',
      license: 'MIT', releases: [{ n: 1, date: '2 months ago' }, { n: 2, date: '3 weeks ago' }],
      addedBy: 'josh-padnick', addedOn: '5 Aug 2026', usedBy: 12,
      rules: [
        { slug: 'document-why-not-what', lang: 'plaintext', group: 'practices/documentation', title: 'Document why, not what', impact: 'LOW-MEDIUM', tags: [], usedBy: 12, net30: 2, stars: 18,
          whenToRead: 'When writing READMEs or design notes.',
          body: simple('Explain decisions and constraints; the code already shows what it does.', 'This function loops over users and sends emails.', 'We batch emails in groups of 100 because the provider rate-limits per connection.', 'Every doc section answers a "why" the code cannot.'),
          added: 1, changes: [] },
        { slug: 'keep-readme-runnable', lang: 'bash', group: 'practices/documentation', title: 'Keep README commands runnable', impact: 'MEDIUM', tags: [], usedBy: 9, net30: 1, stars: 13,
          whenToRead: 'When changing commands, scripts, or setup steps.',
          body: simple('Every command in the README works when pasted into a fresh clone.', 'npm run start:dev  # removed last month', 'npm run dev', 'Run each README command in a clean checkout.'),
          added: 1, changes: [] },
        { slug: 'small-prs-from-agents', lang: 'plaintext', group: 'practices/agent-hygiene', title: 'Keep agent pull requests small', impact: 'LOW-MEDIUM', tags: ['agents'], usedBy: 6, net30: 1, stars: 4,
          whenToRead: 'When an agent is about to open a pull request.',
          body: simple('Split agent work into pull requests a person can review in one sitting.', 'One PR: refactor + feature + dependency bump (2,400 lines).', 'Three PRs: refactor, then feature, then dependency bump.', 'Each PR changes one thing and stays under a few hundred lines.'),
          added: 2, addedSummary: 'Add rules for agent pull requests and TypeScript types.', changes: [] },
        { slug: 'prefer-type-aliases', group: 'techs/typescript', title: 'Prefer type aliases for object shapes', impact: 'LOW', tags: [], usedBy: 7, net30: 0, stars: 5,
          whenToRead: 'When declaring object types in TypeScript.',
          body: simple('Use <code>type</code> for object shapes unless you need declaration merging.', 'interface User { id: string }', 'type User = { id: string };', 'New object types use type aliases.'),
          added: 2, addedSummary: 'Add rules for agent pull requests and TypeScript types.', changes: [] },
      ],
      insights: [
        { rule: 'prefer-type-aliases', using: 7, ever: 9, removed: 2, reason: 'No reason recorded (2 projects stopped selecting techs/typescript)' },
      ],
    },
  ];

  // Owners with real: true use their actual GitHub avatar; invented owners get a generated identicon.
  // Libraries the signed-in user could publish, keyed by repo. Private ones only appear after connecting private repos.
  const publishable = [
    { id: 'josh-padnick/rules-experimental', owner: 'josh-padnick', visibility: 'public', groups: 1,
      library: {
        id: 'josh-padnick/rules-experimental', owner: 'josh-padnick', name: 'Experimental Rules', description: "Rules I'm trying out before promoting them.",
        license: 'MIT', releases: [{ n: 1, date: 'yesterday' }], usedBy: 0,
        rules: [
          { slug: 'review-plan-before-diff', lang: 'plaintext', group: 'practices/code-review', title: 'Review the plan before the diff', impact: 'MEDIUM', tags: ['agents'], usedBy: 0, net30: 0, stars: 0,
            whenToRead: 'When reviewing an agent-written pull request.',
            body: simple('Read the stated plan and check it solves the right problem before reading line by line.', 'Start at file 1, line 1.', 'Read the PR description and plan; confirm the approach; then read the diff.', 'Review comments address the approach before style.'),
            added: 1, changes: [] },
          { slug: 'flag-unverified-claims', lang: 'plaintext', group: 'practices/code-review', title: 'Flag unverified claims in PR descriptions', impact: 'MEDIUM', tags: ['agents'], usedBy: 0, net30: 0, stars: 0,
            whenToRead: 'When a PR description says something was tested or verified.',
            body: simple('Ask for evidence when a description claims tests pass or behavior was verified.', '"All tests pass." (no CI run linked)', '"All tests pass: CI run #812. Verified manually in staging: screenshot attached."', 'Every verification claim links to evidence.'),
            added: 1, changes: [] },
        ],
      } },
    { id: 'josh-padnick/team-rules', owner: 'josh-padnick', visibility: 'private', groups: 3 },
  ];

  // Projects that use Code Rules, as Rulemart would read them from provenance.json.
  const myProjects = {
    public: [
      { repo: 'josh-padnick/api-server', sources: [{ lib: 'fabricahq/public-rules', updates: 2, groups: 3 }, { lib: 'gopherworks/go-rules', updates: 0, groups: 1 }] },
      { repo: 'josh-padnick/site', sources: [{ lib: 'fabricahq/public-rules', updates: 0, groups: 2 }] },
    ],
    private: [
      { repo: 'josh-padnick/billing-service', sources: [{ lib: 'fabricahq/public-rules', updates: 0, groups: 4 }, { lib: 'acme/.code-rules', updates: 0, groups: 2 }] },
      { repo: 'josh-padnick/mobile-app', sources: [{ lib: 'fabricahq/public-rules', updates: 1, groups: 3 }] },
    ],
  };

  const projectPool = ['lumen-labs/api', 'openledger/sdk-ts', 'tidepool/web', 'harbor-io/platform', 'quill-app/editor', 'sparrow/billing', 'kitefly/mobile-api', 'northwind/fleet', 'orbit-hq/console', 'mesa-data/pipeline', 'fernwood/cms', 'bramble/checkout'];

  return { groups, owners, user, libraries, publishable, myProjects, projectPool };
})();
