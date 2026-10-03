#!/usr/bin/env bash
# Screenshots every route in the conformance matrix of _internal/realignment.md on the prototype and on the site, side
# by side, so a person can check the site still matches the prototype after a change. It runs on demand, not in CI,
# since it needs both servers and a browser.
#
# Usage:
#
#   _internal/audit/conformance.sh <prototype base URL> <site base URL> <output directory>
#
# for example, with the prototype served as _internal/realignment.md says and the site from make web-dev, with both
# real libraries ingested, as CONTRIBUTING.md says:
#
#   _internal/audit/conformance.sh http://127.0.0.1:8766 http://127.0.0.1:8080 /tmp/rulemart-audit
#
# It needs chrome-devtools-axi, which drives a headless Chrome of its own, and a site built with the rulemartdev tag, so
# it can sign in as test_user. For each route it writes <name>.<width>.<scheme>.prototype.png and
# <name>.<width>.<scheme>.site.png, full-page, at 1280 and 390 pixels wide, light and dark, and index.html, which shows
# each pair side by side. Routes only one of the two has, such as the site's /about, have one screenshot. It stops at
# the first page that doesn't load at the address asked for, with the HTTP status expected, or whose screenshot isn't
# written, naming the route and quoting the browser tool's output.
#
# Set ONLY to an extended regular expression to shoot only the routes whose names match it, such as ONLY='^(rule|me)'
# after changing those pages.
#
# The prototype keeps its state in localStorage, so the script signs it in by writing that state; the site is signed
# in through its dev sign-in, as test_user, and signed out by clearing its cookies. Both carts are filled by writing
# their localStorage keys, with rules each catalog has.

set -euo pipefail

if [[ $# -ne 3 ]]; then
  sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi
proto=${1%/}
site=${2%/}
out=$3
mkdir -p "$out"

# Its own browser session, so the audit never drives a browser another task uses.
export CHROME_DEVTOOLS_AXI_SESSION=${CHROME_DEVTOOLS_AXI_SESSION:-rulemart-audit}

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

# js prints what a JavaScript expression returns, which must be a string. A script that throws, or returns anything
# else, stops the audit, since the tool reports a thrown error as the expression's result.
js() {
  axi eval "$1"
  local result
  result=$(sed -n 's/^result: //p' <<<"$axi_out")
  jq -er 'fromjson | strings' <<<"$result" 2>/dev/null || die "the script $1 didn't return a string:"$'\n'"$axi_out"
}

# visit opens a URL and checks the page the browser landed on: its address must be the one asked for, so a navigation
# the tool dropped, which leaves about:blank, or a browser error page stops the audit, and the response's HTTP status
# must be the one expected.
visit() {
  local url=$1 status=$2 landed
  axi open "$url"
  axi wait 700
  landed=$(js '(() => { const n = performance.getEntriesByType("navigation")[0]; return JSON.stringify({address: location.href, status: n ? n.responseStatus : 0}) })()')
  local address code
  address=$(jq -r .address <<<"$landed")
  code=$(jq -r .status <<<"$landed")
  [[ $address == "$url" ]] || die "the browser is on $address, not $url"
  [[ $code == "$status" ]] || die "$url answered HTTP $code, not $status"
}

# The routes, one per line: a name, the prototype's hash route or - when it has none, the site's path or - when it has
# none, whether to visit them signed out or signed in, and the HTTP status the site answers with. The prototype's mock data and the site's real libraries hold
# different rules, so each pair names the closest equivalents: a rule with versions, a rule with assets, a retired rule.
routes=$(cat <<'EOF'
home                    #/                                                                                  /                                                                                         out 200
browse-techs            #/browse/techs                                                                      /browse/techs                                                                             out 200
browse-practices        #/browse/practices                                                                  /browse/practices                                                                         out 200
browse-techs-other      #/browse/techs/other                                                                /browse/techs/other                                                                       out 200
browse-practices-other  #/browse/practices/other                                                            /browse/practices/other                                                                   out 200
libraries               #/libraries                                                                         /libraries                                                                                out 200
owner                   #/fabricahq                                                                         /fabricahq                                                                                out 200
faq                     #/faq                                                                               /faq                                                                                      out 200
feedback                #/feedback                                                                          /feedback                                                                                 out 200
group                   #/g/techs/go                                                                        /g/techs/go                                                                               out 200
group-practice          #/g/practices/testing                                                               /g/practices/testing                                                                      out 200
search                  #/search?q=retry                                                                    /search?q=retry                                                                           out 200
library                 #/fabricahq/public-rules                                                            /fabricahq/public-rules                                                                   out 200
library-rules           #/fabricahq/public-rules?tab=rules                                                  /fabricahq/public-rules?tab=rules                                                         out 200
library-releases        #/fabricahq/public-rules?tab=releases                                               /fabricahq/code-rules-test-library?tab=releases                                           out 200
library-group           #/fabricahq/public-rules/techs/go                                                   /fabricahq/public-rules/techs/go                                                          out 200
rule                    #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints     /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits                  out 200
rule-versions           #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits?tab=versions out 200
rule-compare            #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions&compare=1.1.0...2.0.0 /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits?tab=versions&from=2.1.0&to=3.0.0 out 200
rule-assets             #/fabricahq/public-rules/practices/testing/test-changed-behavior                    /fabricahq/code-rules-test-library/techs/go/use-contexts                                  out 200
asset                   #/fabricahq/public-rules/practices/testing/test-changed-behavior/assets/why-revert-check.md /fabricahq/code-rules-test-library/techs/go/use-contexts/assets/example.go   out 200
rule-retired            #/fabricahq/public-rules/practices/testing/check-retry-limits                       /fabricahq/code-rules-test-library/practices/testing/verify-retries                       out 200
cart                    #/cart                                                                              /cart                                                                                     out 200
signin                  #/signin                                                                            /signin                                                                                   out 200
about                   -                                                                                   /about                                                                                    out 200
privacy                 -                                                                                   /privacy                                                                                  out 200
missing                 #/nothing/here/at/all/x/y                                                           /nothing/here/at/all/x/y                                                                  out 404
home-signed-in          #/                                                                                  /                                                                                         in 200
rule-signed-in          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints     /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits                  in 200
me                      #/me                                                                                /me                                                                                       in 200
me-stars                #/me?tab=stars                                                                      /me?tab=stars                                                                             in 200
me-add                  #/me/add                                                                            /me/add                                                                                   in 200
me-add-run              #/me/add/run?repo=josh-padnick/rules-experimental                                   /me/add/run?repo=fabricahq/code-rules-test-library                                        in 200
me-private              #/me/private                                                                        /me/private                                                                               in 200
cart-signed-in          #/cart                                                                              /cart                                                                                     in 200
EOF
)

# What each cart holds: a rule and a whole group, by the keys each cart keeps.
proto_cart='["fabricahq/public-rules::practices/testing/verify-retry-limits","group::fabricahq/public-rules::techs/go"]'
site_cart='["fabricahq/code-rules-test-library::practices/testing/verify-retry-limits","group::fabricahq/public-rules::techs/go"]'

# set_state signs both in or out, and fills both carts, then leaves the browser on the site.
set_state() {
  local signed_in=$1
  doing="signing the prototype ${2}"
  visit "$proto/" 200
  [[ $(js "localStorage.setItem('rulemart-mock-v1', JSON.stringify({signedIn: $signed_in, cart: $proto_cart})), 'ok'") == ok ]]
  doing="signing the site ${2}"
  visit "$site/" 200
  [[ $(js "localStorage.setItem('rulemart-cart', JSON.stringify({cart: $site_cart})), 'ok'") == ok ]]
  if [[ $signed_in == true ]]; then
    visit "$site/signin" 200
    [[ $(js "document.querySelector('form[action*=\"dev-sign-in\"][action*=\"as=test_user&\"]').submit(), 'ok'") == ok ]]
    axi wait 1500
  else
    # The session cookie is HttpOnly, so sign out through the site itself when a sign-out form is on the page.
    [[ $(js "(() => { const f = document.querySelector('form[action^=\"/signout\"]'); if (f) f.submit(); return 'ok' })()") == ok ]]
    axi wait 1000
  fi
}

# shoot visits a URL and writes a full-page screenshot of it to a file, which must then exist and hold an image.
shoot() {
  local url=$1 status=$2 file=$3
  rm -f "$file"
  visit "$url" "$status"
  axi screenshot "$file" --full-page
  [[ -s $file ]] || die "the screenshot of $url wasn't written to $file:"$'\n'"$axi_out"
}

index="$out/index.html"
{
  echo '<!doctype html><meta charset="utf-8"><title>Rulemart conformance audit</title>'
  echo '<style>body{font:14px system-ui;margin:16px}h2{margin:32px 0 8px;font-size:15px}.pair{display:grid;grid-template-columns:1fr 1fr;gap:12px;align-items:start}img{width:100%;border:1px solid #ccc}.narrow img{max-width:390px}</style>'
  echo "<h1>Rulemart conformance audit</h1><p>Prototype $proto, left; site $site, right.</p>"
} >"$index"

for state in out in; do
  if [[ $state == in ]]; then set_state true in; else set_state false out; fi
  for width in 1280 390; do
    for scheme in light dark; do
      if [[ $width == 390 ]]; then viewport="390x844x2,mobile,touch"; else viewport="1280x900x1"; fi
      doing="emulating $width px, $scheme"
      axi emulate --viewport "$viewport" --color-scheme "$scheme"
      while read -r name proto_route site_route visit status; do
        [[ -z $name || $visit != "$state" ]] && continue
        [[ -n ${ONLY:-} && ! $name =~ $ONLY ]] && continue
        base="$name.$width.$scheme"
        echo "$base"
        doing="shooting $base, signed $state"
        cells=""
        if [[ $proto_route != - ]]; then
          shoot "$proto/$proto_route" 200 "$out/$base.prototype.png"
          cells+="<img src=\"$base.prototype.png\" alt=\"Prototype\">"
        else
          cells+="<p>No prototype route.</p>"
        fi
        if [[ $site_route != - ]]; then
          shoot "$site$site_route" "$status" "$out/$base.site.png"
          cells+="<img src=\"$base.site.png\" alt=\"Site\">"
        fi
        class=""; [[ $width == 390 ]] && class=" narrow"
        echo "<h2 id=\"$base\">$name, $width px, $scheme, signed $state</h2><div class=\"pair$class\">$cells</div>" >>"$index"
      done <<<"$routes"
    done
  done
done

echo "Wrote $index"
