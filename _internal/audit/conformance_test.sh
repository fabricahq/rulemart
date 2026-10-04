#!/usr/bin/env bash
# Checks that conformance.sh drives a disposable browser of its own whatever the environment it inherits: it must start
# chrome-devtools-axi with a session named for the run and without any setting that connects to an existing browser or
# keeps a profile, so the audit can't sign another browser's visitor out or overwrite its cart and theme. A stand-in
# chrome-devtools-axi records the settings it gets and fails, which stops the audit at its first browser command. make
# check runs it.
#
# Usage:
#
#   _internal/audit/conformance_test.sh

set -euo pipefail

if ! bash -c '((BASH_VERSINFO[0] >= 4))'; then
  echo "conformance_test.sh: skipped: conformance.sh needs bash 4 or later, such as Homebrew's"
  exit 0
fi

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
mkdir "$dir/bin"
cat >"$dir/bin/chrome-devtools-axi" <<'STUB'
#!/usr/bin/env bash
{ echo "call $1"; env | grep '^CHROME_DEVTOOLS_AXI_' | sort; } >>"$AXI_RECORD"
echo "error: the stand-in browser doesn't open pages"
exit 1
STUB
chmod +x "$dir/bin/chrome-devtools-axi"

status=0
env PATH="$dir/bin:$PATH" AXI_RECORD="$dir/record" \
  CHROME_DEVTOOLS_AXI_AUTO_CONNECT=1 \
  CHROME_DEVTOOLS_AXI_BROWSER_URL=http://127.0.0.1:9222 \
  CHROME_DEVTOOLS_AXI_WS_HEADERS='{"Authorization":"Bearer x"}' \
  CHROME_DEVTOOLS_AXI_MCP_SERVER_URL=http://127.0.0.1:9333/mcp \
  CHROME_DEVTOOLS_AXI_USER_DATA_DIR="$dir/profile" \
  CHROME_DEVTOOLS_AXI_PORT=9224 \
  CHROME_DEVTOOLS_AXI_CHROME_ARGS=--user-data-dir="$dir/profile" \
  CHROME_DEVTOOLS_AXI_SESSION=default \
  "$(dirname "$0")/conformance.sh" http://127.0.0.1:1 http://127.0.0.1:2 "$dir/out" >"$dir/output" 2>&1 || status=$?

fail() {
  echo "conformance_test.sh: $1" >&2
  echo "--- conformance.sh's output:" >&2
  cat "$dir/output" >&2
  echo "--- what chrome-devtools-axi got:" >&2
  cat "$dir/record" >&2 2>/dev/null || true
  exit 1
}

((status == 1)) || fail "conformance.sh exited $status, want 1, stopped by the stand-in's failure"
grep -q '^call open$' "$dir/record" 2>/dev/null || fail "conformance.sh never opened a page"
if grep -qE '^CHROME_DEVTOOLS_AXI_(AUTO_CONNECT|BROWSER_URL|WS_HEADERS|MCP_SERVER_URL|USER_DATA_DIR|PORT|CHROME_ARGS)=' "$dir/record"; then
  fail "conformance.sh passed on a setting that connects to an existing browser or keeps a profile"
fi
if grep '^CHROME_DEVTOOLS_AXI_SESSION=' "$dir/record" | grep -qv '^CHROME_DEVTOOLS_AXI_SESSION=rulemart-audit-'; then
  fail "conformance.sh ran chrome-devtools-axi in a session not named for the run"
fi
grep -q '^call stop$' "$dir/record" || fail "conformance.sh didn't stop its browser as it exited"
echo "conformance_test.sh: ok"
