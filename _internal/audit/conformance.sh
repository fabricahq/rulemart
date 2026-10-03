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
# each pair side by side. Routes only one of the two has, such as the site's /about, have one screenshot.
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
axi() { chrome-devtools-axi "$@" >/dev/null; }

# The routes, one per line: a name, the prototype's hash route or - when it has none, the site's path or - when it has
# none, and whether to visit them signed out or signed in. The prototype's mock data and the site's real libraries hold
# different rules, so each pair names the closest equivalents: a rule with versions, a rule with assets, a retired rule.
routes=$(cat <<'EOF'
home                    #/                                                                                  /                                                                                         out
browse-techs            #/browse/techs                                                                      /browse/techs                                                                             out
browse-practices        #/browse/practices                                                                  /browse/practices                                                                         out
browse-techs-other      #/browse/techs/other                                                                /browse/techs/other                                                                       out
browse-practices-other  #/browse/practices/other                                                            /browse/practices/other                                                                   out
libraries               #/libraries                                                                         /libraries                                                                                out
owner                   #/fabricahq                                                                         /fabricahq                                                                                out
faq                     #/faq                                                                               /faq                                                                                      out
feedback                #/feedback                                                                          /feedback                                                                                 out
group                   #/g/techs/go                                                                        /g/techs/go                                                                               out
group-practice          #/g/practices/testing                                                               /g/practices/testing                                                                      out
search                  #/search?q=retry                                                                    /search?q=retry                                                                           out
library                 #/fabricahq/public-rules                                                            /fabricahq/public-rules                                                                   out
library-rules           #/fabricahq/public-rules?tab=rules                                                  /fabricahq/public-rules?tab=rules                                                         out
library-releases        #/fabricahq/public-rules?tab=releases                                               /fabricahq/code-rules-test-library?tab=releases                                           out
library-group           #/fabricahq/public-rules/techs/go                                                   /fabricahq/public-rules/techs/go                                                          out
rule                    #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints     /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits                  out
rule-versions           #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits?tab=versions out
rule-compare            #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints?tab=versions&compare=1.1.0...2.0.0 /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits?tab=versions&from=2.1.0&to=3.0.0 out
rule-assets             #/fabricahq/public-rules/practices/testing/test-changed-behavior                    /fabricahq/code-rules-test-library/techs/go/use-contexts                                  out
asset                   #/fabricahq/public-rules/practices/testing/test-changed-behavior/assets/why-revert-check.md /fabricahq/code-rules-test-library/techs/go/use-contexts/assets/example.go   out
rule-retired            #/fabricahq/public-rules/practices/testing/check-retry-limits                       /fabricahq/code-rules-test-library/practices/testing/verify-retries                       out
cart                    #/cart                                                                              /cart                                                                                     out
signin                  #/signin                                                                            /signin                                                                                   out
about                   -                                                                                   /about                                                                                    out
privacy                 -                                                                                   /privacy                                                                                  out
missing                 #/nothing/here/at/all/x/y                                                           /nothing/here/at/all/x/y                                                                  out
home-signed-in          #/                                                                                  /                                                                                         in
rule-signed-in          #/fabricahq/public-rules/practices/comments/comment-role-result-and-constraints     /fabricahq/code-rules-test-library/practices/testing/verify-retry-limits                  in
me                      #/me                                                                                /me                                                                                       in
me-stars                #/me?tab=stars                                                                      /me?tab=stars                                                                             in
me-add                  #/me/add                                                                            /me/add                                                                                   in
me-add-run              #/me/add/run?repo=josh-padnick/rules-experimental                                   /me/add/run?repo=fabricahq/code-rules-test-library                                        in
me-private              #/me/private                                                                        /me/private                                                                               in
cart-signed-in          #/cart                                                                              /cart                                                                                     in
EOF
)

# What each cart holds: a rule and a whole group, by the keys each cart keeps.
proto_cart='["fabricahq/public-rules::practices/testing/verify-retry-limits","group::fabricahq/public-rules::techs/go"]'
site_cart='["fabricahq/code-rules-test-library::practices/testing/verify-retry-limits","group::fabricahq/public-rules::techs/go"]'

# set_state signs both in or out, and fills both carts, then leaves the browser on the site.
set_state() {
  local signed_in=$1
  axi open "$proto/"
  axi eval "localStorage.setItem('rulemart-mock-v1', JSON.stringify({signedIn: $signed_in, cart: $proto_cart})), true"
  axi open "$site/"
  axi eval "localStorage.setItem('rulemart-cart', JSON.stringify({cart: $site_cart})), true"
  if [[ $signed_in == true ]]; then
    axi open "$site/signin"
    axi eval "document.querySelector('form[action*=\"dev-sign-in\"][action*=\"as=test_user&\"]').submit(), true"
    axi wait 1500
  else
    # The session cookie is HttpOnly, so sign out through the site itself when a sign-out form is on the page.
    axi eval "(() => { const f = document.querySelector('form[action^=\"/signout\"]'); if (f) f.submit(); return true })()"
    axi wait 1000
  fi
}

shoot() {
  local url=$1 file=$2
  axi open "$url"
  axi wait 700
  chrome-devtools-axi screenshot "$file" --full-page >/dev/null
}

index="$out/index.html"
{
  echo '<!doctype html><meta charset="utf-8"><title>Rulemart conformance audit</title>'
  echo '<style>body{font:14px system-ui;margin:16px}h2{margin:32px 0 8px;font-size:15px}.pair{display:grid;grid-template-columns:1fr 1fr;gap:12px;align-items:start}img{width:100%;border:1px solid #ccc}.narrow img{max-width:390px}</style>'
  echo "<h1>Rulemart conformance audit</h1><p>Prototype $proto, left; site $site, right.</p>"
} >"$index"

for state in out in; do
  if [[ $state == in ]]; then set_state true; else set_state false; fi
  for width in 1280 390; do
    for scheme in light dark; do
      if [[ $width == 390 ]]; then viewport="390x844x2,mobile,touch"; else viewport="1280x900x1"; fi
      axi emulate --viewport "$viewport" --color-scheme "$scheme"
      while read -r name proto_route site_route visit; do
        [[ -z $name || $visit != "$state" ]] && continue
        [[ -n ${ONLY:-} && ! $name =~ $ONLY ]] && continue
        base="$name.$width.$scheme"
        echo "$base"
        cells=""
        if [[ $proto_route != - ]]; then
          shoot "$proto/$proto_route" "$out/$base.prototype.png"
          cells+="<img src=\"$base.prototype.png\" alt=\"Prototype\">"
        else
          cells+="<p>No prototype route.</p>"
        fi
        if [[ $site_route != - ]]; then
          shoot "$site$site_route" "$out/$base.site.png"
          cells+="<img src=\"$base.site.png\" alt=\"Site\">"
        fi
        class=""; [[ $width == 390 ]] && class=" narrow"
        echo "<h2 id=\"$base\">$name, $width px, $scheme, signed $state</h2><div class=\"pair$class\">$cells</div>" >>"$index"
      done <<<"$routes"
    done
  done
done

echo "Wrote $index"
