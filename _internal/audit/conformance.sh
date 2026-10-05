#!/usr/bin/env bash
# Screenshots every route in the conformance matrix of _internal/realignment.md on the prototype and on the site, side
# by side, so a person can check the site still matches the prototype after a change. It runs on demand, not in CI,
# since it needs both servers and a browser.
#
# Usage:
#
#   _internal/audit/conformance.sh <prototype base URL> <site base URL> <output directory>
#
# for example, with the prototype served as _internal/realignment.md says and the site from make web-dev, with
# fabricahq/public-rules ingested and fabricahq/code-rules-test-library vetted locally, as CONTRIBUTING.md says:
#
#   _internal/audit/conformance.sh http://127.0.0.1:8766 http://127.0.0.1:8080 /tmp/rulemart-audit
#
# It needs chrome-devtools-axi, which drives a headless Chrome of its own, and a site built with the rulemartdev tag, so
# it can sign in as test_user. Each case is a route, signed out or in, with a dialog or box opened or not, as the table
# below lists them. For each it writes <name>.signed-<out|in>.<width>.<scheme>.prototype.png and the same .site.png,
# full-page, at 1280 and 390 pixels wide, light and dark, and index.html, which shows each pair side by side. Routes
# only one of the two has, such as the site's /about and /about/vetting, have one screenshot. It stops at the first page that doesn't load
# at the address expected, with the HTTP status expected, whose interaction doesn't open, or whose screenshot isn't
# written, naming the case and quoting the browser tool's output.
#
# Set ONLY to an extended regular expression to shoot only the cases whose names match it, such as ONLY='^(rule|me)'
# after changing those pages.
#
# The prototype keeps its state in localStorage, so the script signs it in by writing that state; the site is signed
# in through its dev sign-in, as test_user, and signed out through its sign-out form. Both carts are filled by writing
# their localStorage keys, with rules each catalog has. Each run uses a browser of its own, which it stops as it exits,
# signing the site out first, and before each screenshot it checks the page is signed in or out as the case says and
# keeps no saved theme, so the emulated color scheme applies.

set -euo pipefail

# The cases' interactions are associative arrays, which macOS's own bash 3 lacks.
((BASH_VERSINFO[0] >= 4)) || { echo "conformance.sh needs bash 4 or later, such as Homebrew's" >&2; exit 2; }

if [[ $# -ne 3 ]]; then
  sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi
proto=${1%/}
site=${2%/}
out=$3
mkdir -p "$out"

# A browser of its own, which starts with no cookies, storage, or saved theme and is discarded as the audit exits, so it
# never signs another browser's visitor out or overwrites its cart or theme. The session, named for this run alone,
# gives it a bridge and port of its own, and chrome-devtools-axi launches it with a temporary profile unless a setting
# says otherwise, so the audit ignores every setting that would: connecting to a running browser or a shared browser
# service, a profile kept on disk, a fixed bridge port another session may hold, and Chrome flags, which can name a
# profile or a port too.
# conformance_test.sh checks it.
for setting in CHROME_DEVTOOLS_AXI_{AUTO_CONNECT,BROWSER_URL,WS_HEADERS,MCP_SERVER_URL,USER_DATA_DIR,PORT,CHROME_ARGS}; do
  if [[ -n ${!setting+set} ]]; then
    echo "conformance.sh: ignoring $setting, since the audit runs a browser of its own" >&2
    unset "$setting"
  fi
done
export CHROME_DEVTOOLS_AXI_SESSION="rulemart-audit-$$-$RANDOM"

# doing names what the audit is doing, for the message that stops it.
doing="setting up"
die() {
  echo "conformance.sh: $doing: $1" >&2
  exit 1
}

# axi runs one chrome-devtools-axi command and keeps its output in axi_out. The tool reports some failures only in its
# output, with exit status 0, such as an error: line, so the audit stops on either, with the tool's output.
axi() {
  local status=0
  axi_out=$(chrome-devtools-axi "$@" 2>&1) || status=$?
  if ((status != 0)) || grep -q '^error:' <<<"$axi_out"; then
    die "chrome-devtools-axi $1 failed with exit status $status:"$'\n'"$axi_out"
  fi
}

# signed_in is the state the site is in, which cleanup undoes: true once the audit signed it in.
signed_in=false

# cleanup runs as the audit exits, whether it finished or stopped: it signs the site out, so its session ends, and stops
# the run's browser, which discards its profile.
cleanup() {
  if [[ $signed_in == true ]]; then
    chrome-devtools-axi open "$site/" >/dev/null 2>&1 &&
      chrome-devtools-axi eval "(() => { const f = document.querySelector('form[action^=\"/signout\"]'); if (f) f.submit(); return 'ok' })()" >/dev/null 2>&1 &&
      chrome-devtools-axi wait 1000 >/dev/null 2>&1 || true
  fi
  chrome-devtools-axi stop >/dev/null 2>&1 || true
}
trap cleanup EXIT

# js prints what a JavaScript expression returns, which must be a string. A script that throws, or returns anything
# else, stops the audit, since the tool reports a thrown error as the expression's result.
js() {
  axi eval "$1"
  local result
  result=$(sed -n 's/^result: //p' <<<"$axi_out")
  jq -er 'fromjson | strings' <<<"$result" 2>/dev/null || die "the script $1 didn't return a string:"$'\n'"$axi_out"
}

# visit opens a URL and checks the page the browser landed on: its address must be the one asked for, or landing, a
# path on the same origin, when that isn't =, so a navigation the tool dropped, which leaves about:blank, or a browser
# error page stops the audit, and the response's HTTP status must be the one expected.
visit() {
  local url=$1 status=$2 landing=${3:-=} landed
  axi open "$url"
  axi wait 700
  landed=$(js '(() => { const n = performance.getEntriesByType("navigation")[0]; return JSON.stringify({address: location.href, status: n ? n.responseStatus : 0}) })()')
  local address code
  address=$(jq -r .address <<<"$landed")
  code=$(jq -r .status <<<"$landed")
  local want=$url
  [[ $landing == = ]] || want="${url%%://*}://$(cut -d/ -f3 <<<"$url")$landing"
  [[ $address == "$want" ]] || die "the browser is on $address, not $want"
  [[ $code == "$status" ]] || die "$url answered HTTP $code, not $status"
}

# The cases, one per line: a name; whether to visit signed out or in; what to open on the page first, - for nothing,
# star for the dialog Star opens for a visitor who isn't signed in, add for the Add to cart dialog, or newproject for
# checkout's box for a project that doesn't use Code Rules yet, beside its project picker; the prototype's hash route,
# or - when it has none; the site's path, or - when it has none; the site's address once it has redirected, or = when
# it doesn't; and the HTTP status the site answers with. The prototype's mock data and the site's real libraries hold
# different rules, so each pair names the closest equivalents: a rule with versions, a rule with assets, a retired
# rule. The prototype has no comparison of library releases, so the site's is beside the prototype's releases tab.
#
# The site's cases open fabricahq/public-rules, which Rulemart vets, except where only the test library,
# fabricahq/code-rules-test-library, has what the case shows: public-rules has one release, so each of its rules has
# one version and none is retired, and library-compare, rule-compare, and rule-retired open the test library. Rulemart doesn't
# vet it, so those cases answer 404, and stop the audit, unless it's vetted locally, in a change to catalog/vetted.yaml
# that isn't committed, as CONTRIBUTING.md says. Listing it at /me/add loads them too, but under the unvetted warning,
# which the prototype doesn't show.
cases=$(cat <<'EOF'
home                   out -          #/                                                                                                                 /                                                                                                         =                                  200
browse-techs           out -          #/browse/techs                                                                                                     /browse/techs                                                                                             =                                  200
browse-practices       out -          #/browse/practices                                                                                                 /browse/practices                                                                                         =                                  200
browse-techs-other     out -          #/browse/techs/other                                                                                               /browse/techs/other                                                                                       =                                  200
browse-practices-other out -          #/browse/practices/other                                                                                           /browse/practices/other                                                                                   =                                  200
libraries              out -          #/libraries                                                                                                        /libraries                                                                                                =                                  200
owner                  out -          #/fabricahq                                                                                                        /fabricahq                                                                                                =                                  200
faq                    out -          #/faq                                                                                                              /faq                                                                                                      =                                  200
feedback               out -          #/feedback                                                                                                         /feedback                                                                                                 =                                  200
group                  out -          #/g/techs/go                                                                                                       /g/techs/go                                                                                               =                                  200
group-practice         out -          #/g/practices/testing                                                                                              /g/practices/testing                                                                                      =                                  200
search                 out -          #/search?q=retry                                                                                                   /search?q=retry                                                                                           =                                  200
library                out -          #/fabricahq/public-rules                                                                                           /fabricahq/public-rules                                                                                   =                                  200
library-rules          out -          #/fabricahq/public-rules?tab=rules                                                                                 /fabricahq/public-rules?tab=rules                                                                         =                                  200
library-releases       out -          #/fabricahq/public-rules?tab=releases                                                                              /fabricahq/public-rules?tab=releases                                                                      =                                  200
library-compare        out -          #/fabricahq/public-rules?tab=releases                                                                              /fabricahq/code-rules-test-library?tab=releases&from=2&to=4                                               =                                  200
library-group          out -          #/fabricahq/public-rules/techs/go                                                                                  /fabricahq/public-rules/techs/go                                                                          =                                  200
rule                   out -          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints                                    /fabricahq/public-rules/practices/testing/keep-tests-independent                                          =                                  200
rule-star-dialog       out star       #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints                                    /fabricahq/public-rules/practices/testing/keep-tests-independent                                          =                                  200
rule-add-dialog        out add        #/fabricahq/public-rules/practices/testing/test-changed-behavior                                                   /fabricahq/public-rules/practices/testing/test-observable-behavior                                        =                                  200
rule-versions          out -          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions                       /fabricahq/public-rules/practices/testing/keep-tests-independent?tab=versions                             =                                  200
rule-compare           out -          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions&compare=1.1.0...2.0.0 /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits?tab=versions&from=2.1.0&to=3.0.0 =                                  200
rule-assets            out -          #/fabricahq/public-rules/practices/testing/test-changed-behavior                                                   /fabricahq/public-rules/practices/testing/test-observable-behavior                                        =                                  200
asset                  out -          #/fabricahq/public-rules/practices/testing/test-changed-behavior/assets/why-revert-check.md                        /fabricahq/public-rules/assets/testing-philosophy.md?rule=practices/testing/test-observable-behavior      =                                  200
rule-retired           out -          #/fabricahq/public-rules/practices/testing/check-retry-limits                                                      /fabricahq/code-rules-test-library/practices/testing/verify-retries                                       =                                  200
cart                   out -          #/cart                                                                                                             /cart                                                                                                     =                                  200
about                  out -          -                                                                                                                  /about                                                                                                    =                                  200
about-vetting          out -          -                                                                                                                  /about/vetting                                                                                            =                                  200
privacy                out -          -                                                                                                                  /privacy                                                                                                  =                                  200
missing                out -          #/nothing/here/at/all/x/y                                                                                          /nothing/here/at/all/x/y                                                                                  =                                  404
signin                 out -          #/signin                                                                                                           /signin                                                                                                   =                                  200
me                     out -          #/me                                                                                                               /me                                                                                                       /signin?return=%2Fme               200
me-stars               out -          #/me?tab=stars                                                                                                     /me?tab=stars                                                                                             /signin?return=%2Fme%3Ftab%3Dstars 200
me-add                 out -          #/me/add                                                                                                           /me/add                                                                                                   /signin?return=%2Fme%2Fadd         200
me-private             out -          #/me/private                                                                                                       /me/private                                                                                               /signin?return=%2Fme%2Fprivate     200
home                   in  -          #/                                                                                                                 /                                                                                                         =                                  200
browse-techs           in  -          #/browse/techs                                                                                                     /browse/techs                                                                                             =                                  200
browse-practices       in  -          #/browse/practices                                                                                                 /browse/practices                                                                                         =                                  200
browse-techs-other     in  -          #/browse/techs/other                                                                                               /browse/techs/other                                                                                       =                                  200
browse-practices-other in  -          #/browse/practices/other                                                                                           /browse/practices/other                                                                                   =                                  200
libraries              in  -          #/libraries                                                                                                        /libraries                                                                                                =                                  200
owner                  in  -          #/fabricahq                                                                                                        /fabricahq                                                                                                =                                  200
faq                    in  -          #/faq                                                                                                              /faq                                                                                                      =                                  200
feedback               in  -          #/feedback                                                                                                         /feedback                                                                                                 =                                  200
group                  in  -          #/g/techs/go                                                                                                       /g/techs/go                                                                                               =                                  200
group-practice         in  -          #/g/practices/testing                                                                                              /g/practices/testing                                                                                      =                                  200
search                 in  -          #/search?q=retry                                                                                                   /search?q=retry                                                                                           =                                  200
group-mine             in  -          #/g/techs/go?mine=1                                                                                                /g/techs/go?mine=1                                                                                        =                                  200
search-mine            in  -          #/search?q=retry&mine=1                                                                                            /search?mine=1&q=retry                                                                                    =                                  200
library                in  -          #/fabricahq/public-rules                                                                                           /fabricahq/public-rules                                                                                   =                                  200
library-rules          in  -          #/fabricahq/public-rules?tab=rules                                                                                 /fabricahq/public-rules?tab=rules                                                                         =                                  200
library-releases       in  -          #/fabricahq/public-rules?tab=releases                                                                              /fabricahq/public-rules?tab=releases                                                                      =                                  200
library-compare        in  -          #/fabricahq/public-rules?tab=releases                                                                              /fabricahq/code-rules-test-library?tab=releases&from=2&to=4                                               =                                  200
library-group          in  -          #/fabricahq/public-rules/techs/go                                                                                  /fabricahq/public-rules/techs/go                                                                          =                                  200
rule                   in  -          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints                                    /fabricahq/public-rules/practices/testing/keep-tests-independent                                          =                                  200
rule-add-dialog        in  add        #/fabricahq/public-rules/practices/testing/test-changed-behavior                                                   /fabricahq/public-rules/practices/testing/test-observable-behavior                                        =                                  200
rule-versions          in  -          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions                       /fabricahq/public-rules/practices/testing/keep-tests-independent?tab=versions                             =                                  200
rule-compare           in  -          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions&compare=1.1.0...2.0.0 /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits?tab=versions&from=2.1.0&to=3.0.0 =                                  200
rule-assets            in  -          #/fabricahq/public-rules/practices/testing/test-changed-behavior                                                   /fabricahq/public-rules/practices/testing/test-observable-behavior                                        =                                  200
asset                  in  -          #/fabricahq/public-rules/practices/testing/test-changed-behavior/assets/why-revert-check.md                        /fabricahq/public-rules/assets/testing-philosophy.md?rule=practices/testing/test-observable-behavior      =                                  200
rule-retired           in  -          #/fabricahq/public-rules/practices/testing/check-retry-limits                                                      /fabricahq/code-rules-test-library/practices/testing/verify-retries                                       =                                  200
cart                   in  -          #/cart                                                                                                             /cart                                                                                                     =                                  200
cart-new-project       in  newproject #/cart                                                                                                             /cart                                                                                                     =                                  200
about                  in  -          -                                                                                                                  /about                                                                                                    =                                  200
about-vetting          in  -          -                                                                                                                  /about/vetting                                                                                            =                                  200
privacy                in  -          -                                                                                                                  /privacy                                                                                                  =                                  200
missing                in  -          #/nothing/here/at/all/x/y                                                                                          /nothing/here/at/all/x/y                                                                                  =                                  404
me                     in  -          #/me                                                                                                               /me                                                                                                       =                                  200
me-stars               in  -          #/me?tab=stars                                                                                                     /me?tab=stars                                                                                             =                                  200
me-add                 in  -          #/me/add                                                                                                           /me/add                                                                                                   =                                  200
me-add-run             in  -          #/me/add/run?repo=josh-padnick/rules-experimental                                                                  /me/add/run?repo=fabricahq/public-rules                                                                   =                                  200
me-private             in  -          #/me/private                                                                                                       /me/private                                                                                               =                                  200
EOF
)

# What each cart holds: a rule and a whole group, by the keys each cart keeps.
proto_cart='["fabricahq/public-rules::practices/testing/verify-retry-limits","group::fabricahq/public-rules::techs/go"]'
site_cart='["fabricahq/public-rules::practices/testing/keep-tests-independent","group::fabricahq/public-rules::techs/go"]'

# set_state signs both in or out, empties each one's saved theme, so the emulated color scheme decides, and fills both
# carts, then checks each is in that state.
set_state() {
  local want=$1
  doing="signing the prototype ${2}"
  visit "$proto/" 200
  # The prototype keeps everything, its theme too, in one key, so writing it whole leaves its theme at system.
  [[ $(js "localStorage.setItem('rulemart-mock-v1', JSON.stringify({signedIn: $want, cart: $proto_cart})), 'ok'") == ok ]]
  doing="signing the site ${2}"
  visit "$site/" 200
  [[ $(js "localStorage.removeItem('rulemart-theme'), localStorage.setItem('rulemart-cart', JSON.stringify({cart: $site_cart})), 'ok'") == ok ]]
  if [[ $want == true ]]; then
    visit "$site/signin" 200
    signed_in=true
    [[ $(js "document.querySelector('form[action*=\"dev-sign-in\"][action*=\"as=test_user&\"]').submit(), 'ok'") == ok ]]
    axi wait 1500
    visit "$site/me" 200
  else
    # The session cookie is HttpOnly, so sign out through the site itself when a sign-out form is on the page.
    [[ $(js "(() => { const f = document.querySelector('form[action^=\"/signout\"]'); if (f) f.submit(); return 'ok' })()") == ok ]]
    axi wait 1000
    signed_in=false
    visit "$site/me" 200 "/signin?return=%2Fme"
  fi
  check_state site "$want"
  visit "$proto/" 200
  check_state prototype "$want"
}

# check_state stops the audit unless the page open on side, prototype or site, is signed in when want is true and
# signed out otherwise, and keeps no theme of its own, so the color scheme the browser emulates, scheme, applies.
check_state() {
  local side=$1 want=$2 got
  if [[ $side == prototype ]]; then
    got=$(js "String(JSON.parse(localStorage.getItem('rulemart-mock-v1') || '{}').signedIn === true)")
  else
    got=$(js "String(document.querySelector('form[action^=\"/signout\"]') !== null)")
  fi
  [[ $got == "$want" ]] || die "the $side is signed $([[ $want == true ]] && echo out || echo in), not as expected"
  [[ $(js "document.documentElement.hasAttribute('data-theme') ? 'saved ' + document.documentElement.dataset.theme : 'system'") == system ]] ||
    die "the $side applies a saved theme over the emulated color scheme"
  [[ -z ${scheme:-} || $(js "matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'") == "$scheme" ]] ||
    die "the $side's color scheme isn't $scheme"
}

# The controls each side opens an interaction with, and a script that returns open once the interaction shows, by side
# and interaction. A new project's box stays open in each cart's saved state, so close_controls close it again.
declare -A open_controls=(
  [prototype.star]='[data-act="star"]' [site.star]='a[data-star-signin]'
  [prototype.add]='[data-act="add-to-cart"]' [site.add]='[data-cart-open]'
  [prototype.newproject]='[data-act="cart-newproject"]' [site.newproject]='[data-cart-new-project]'
)
declare -A shown_checks=(
  [prototype.star]='document.querySelector("#modal-root .modal")?.textContent.includes("Sign in to star rules") ? "open" : "closed"'
  [site.star]='document.querySelector("dialog[data-star-dialog]")?.open ? "open" : "closed"'
  [prototype.add]='document.querySelector("#modal-root .modal")?.textContent.includes("Add to cart") ? "open" : "closed"'
  [site.add]='document.querySelector("dialog[data-cart-dialog]")?.open ? "open" : "closed"'
  [prototype.newproject]='[...document.querySelectorAll(".proj-box b")].some((b) => b.textContent === "A new project") ? "open" : "closed"'
  [site.newproject]='document.querySelector("[data-cart-new-box]")?.hidden === false ? "open" : "closed"'
)
declare -A close_controls=([prototype.newproject]='[data-act="cart-existing"]' [site.newproject]='[data-cart-existing]')

# press clicks the control a selector finds, stopping the audit when the page has none.
press() {
  [[ $(js "(() => { const c = document.querySelector('$1'); if (!c) return 'missing'; c.click(); return 'ok' })()") == ok ]] ||
    die "the page has no control $1"
  axi wait 400
}

# shoot visits a URL, opens an interaction on it, unless it's -, and writes a full-page screenshot to a file, which
# must then exist and hold an image. side is prototype or site.
shoot() {
  local side=$1 url=$2 status=$3 landing=$4 interaction=$5 file=$6
  rm -f "$file"
  visit "$url" "$status" "$landing"
  check_state "$side" "$([[ $state == in ]] && echo true || echo false)"
  if [[ $interaction != - ]]; then
    press "${open_controls[$side.$interaction]}"
    [[ $(js "${shown_checks[$side.$interaction]}") == open ]] || die "pressing ${open_controls[$side.$interaction]} didn't open $interaction"
    # Opening the new project's box moves focus to its field, which scrolls the page, and a full-page screenshot then
    # draws the sticky header where the page scrolled to.
    [[ $(js "window.scrollTo(0, 0), 'ok'") == ok ]]
  fi
  axi screenshot "$file" --full-page
  [[ -s $file ]] || die "the screenshot of $url wasn't written to $file:"$'\n'"$axi_out"
  if [[ -n ${close_controls[$side.$interaction]:-} ]]; then
    press "${close_controls[$side.$interaction]}"
    [[ $(js "${shown_checks[$side.$interaction]}") == closed ]] || die "pressing ${close_controls[$side.$interaction]} didn't close $interaction"
  fi
}

index="$out/index.html"
{
  echo '<!doctype html><meta charset="utf-8"><title>Rulemart conformance audit</title>'
  echo '<style>body{font:14px system-ui;margin:16px}h2{margin:32px 0 8px;font-size:15px}.pair{display:grid;grid-template-columns:1fr 1fr;gap:12px;align-items:start}img{width:100%;border:1px solid #ccc}.narrow img{max-width:390px}</style>'
  echo "<h1>Rulemart conformance audit</h1><p>Prototype $proto, left; site $site, right.</p>"
} >"$index"

for state in out in; do
  scheme=""
  if [[ $state == in ]]; then set_state true in; else set_state false out; fi
  for width in 1280 390; do
    for scheme in light dark; do
      if [[ $width == 390 ]]; then viewport="390x844x2,mobile,touch"; else viewport="1280x900x1"; fi
      doing="emulating $width px, $scheme"
      axi emulate --viewport "$viewport" --color-scheme "$scheme"
      while read -r name visit interaction proto_route site_route landing status; do
        [[ -z $name || $visit != "$state" ]] && continue
        [[ -n ${ONLY:-} && ! $name =~ $ONLY ]] && continue
        base="$name.signed-$state.$width.$scheme"
        echo "$base"
        doing="shooting $base"
        cells=""
        if [[ $proto_route != - ]]; then
          shoot prototype "$proto/$proto_route" 200 = "$interaction" "$out/$base.prototype.png"
          cells+="<img src=\"$base.prototype.png\" alt=\"Prototype\">"
        else
          cells+="<p>No prototype route.</p>"
        fi
        if [[ $site_route != - ]]; then
          shoot site "$site$site_route" "$status" "$landing" "$interaction" "$out/$base.site.png"
          cells+="<img src=\"$base.site.png\" alt=\"Site\">"
        fi
        class=""; [[ $width == 390 ]] && class=" narrow"
        opened=""; [[ $interaction != - ]] && opened=", $interaction open"
        echo "<h2 id=\"$base\">$name, signed $state$opened, $width px, $scheme</h2><div class=\"pair$class\">$cells</div>" >>"$index"
      done <<<"$cases"
    done
  done
done

echo "Wrote $index"
