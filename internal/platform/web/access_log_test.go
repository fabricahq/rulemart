package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// loggedSite returns the pages' handler, reading from c, and the buffer its JSON log lines go to. Every request's ID
// is request-123.
func loggedSite(t *testing.T, c web.Catalog) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	handler, err := web.New(c, web.Options{
		Log:       slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestID: func(*http.Request) string { return "request-123" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, &logs
}

// accessLines returns the access log lines in logs, one per request.
func accessLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var found []map[string]any
	for line := range strings.Lines(logs.String()) {
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("a log line isn't JSON: %q", line)
		}
		if fields["msg"] == "request" {
			found = append(found, fields)
		}
	}
	return found
}

// accessLine serves one request through handler and returns the response and its one access log line.
func accessLine(t *testing.T, handler http.Handler, logs *bytes.Buffer, r *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	logs.Reset()
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, r)
	lines := accessLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("%s %s: logged %d access lines, want 1:\n%s", r.Method, r.URL, len(lines), logs)
	}
	return resp, lines[0]
}

// Paths name libraries and rules, so grouping traffic by route needs the pattern a request matched, and a missing
// page must not put the path someone asked for into the logs.
func TestAccessLogRecordsTheRoutePatternNotThePath(t *testing.T) {
	handler, logs := loggedSite(t, newBrowsingCatalog())
	for _, tc := range []struct {
		path, route string
		status      int
	}{
		{"/", "/{$}", http.StatusOK},
		{"/groups", "/groups", http.StatusOK},
		{"/groups/techs/go", "/groups/{kind}/{name}", http.StatusOK},
		{"/groups/techs/golang", "/groups/{kind}/{name}", http.StatusNotFound},
		{"/search?q=errors", "/search", http.StatusOK},
		{"/search?q=private+words", "/search", http.StatusOK},
		{library, "/{owner}/{repo}", http.StatusOK},
		{errorsRule + "?tab=versions", "/{owner}/{repo}/{rule...}", http.StatusOK},
		{"/Example/Rules", "/{owner}/{repo}", http.StatusMovedPermanently},
		{"/example/missing", "/{owner}/{repo}", http.StatusNotFound},
		{"/example/rules/techs/go/missing", "/{owner}/{repo}/{rule...}", http.StatusNotFound},
		{"/nothing-here", "/", http.StatusNotFound},
		{"/_static/000000000000/missing.css", "/_static/{version}/{file...}", http.StatusNotFound},
	} {
		_, line := accessLine(t, handler, logs, httptest.NewRequest(http.MethodGet, tc.path, nil))

		if line["route"] != tc.route || line["status"] != float64(tc.status) || line["method"] != "GET" || line["requestID"] != "request-123" {
			t.Errorf("%s: logged %v, want route %s and status %d", tc.path, line, tc.route, tc.status)
		}
		if _, ok := line["duration_ms"].(float64); !ok {
			t.Errorf("%s: logged no duration_ms: %v", tc.path, line)
		}
	}
	for _, path := range []string{"missing", "nothing-here", "Example", "return-errors", "tab=", "golang", "private", "q="} {
		if strings.Contains(logs.String(), path) {
			t.Errorf("the logs hold the path %q:\n%s", path, logs)
		}
	}
}

// The bytes and Cache-Control logged are what the visitor got, so the logs show what CloudFront could cache and how
// much each route sends.
func TestAccessLogRecordsStatusBytesAndCaching(t *testing.T) {
	handler, logs := loggedSite(t, newCatalog())
	stylesheet := regexp.MustCompile(`href="(/_static/[0-9a-f]+/generated/app\.css)"`).FindStringSubmatch(get(t, handler, "/").Body.String())
	if stylesheet == nil {
		t.Fatal("the page links no stylesheet")
	}
	failing, failingLogs := loggedSite(t, catalog{err: errors.New("database unavailable")})

	for _, tc := range []struct {
		name    string
		handler http.Handler
		logs    *bytes.Buffer
		request *http.Request
		status  int
		cache   string
	}{
		{"a page", handler, logs, httptest.NewRequest(http.MethodGet, errorsRule, nil), http.StatusOK, "public, max-age=60"},
		{"a static file", handler, logs, httptest.NewRequest(http.MethodGet, stylesheet[1], nil), http.StatusOK, "public, max-age=31536000, immutable"},
		{"a failure", failing, failingLogs, httptest.NewRequest(http.MethodGet, library, nil), http.StatusServiceUnavailable, "no-store"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, line := accessLine(t, tc.handler, tc.logs, tc.request)

			if resp.Code != tc.status || resp.Body.Len() == 0 {
				t.Fatalf("answered %d with %d bytes", resp.Code, resp.Body.Len())
			}
			if line["status"] != float64(tc.status) || line["bytes"] != float64(resp.Body.Len()) || line["cache"] != tc.cache {
				t.Fatalf("logged %v, want status %d, %d bytes, and cache %q", line, tc.status, resp.Body.Len(), tc.cache)
			}
		})
	}

	t.Run("a HEAD request", func(t *testing.T) {
		resp, line := accessLine(t, handler, logs, httptest.NewRequest(http.MethodHead, errorsRule, nil))

		if resp.Code != http.StatusOK || line["method"] != "HEAD" || line["status"] != float64(http.StatusOK) || line["bytes"] != 0.0 {
			t.Fatalf("answered %d and logged %v, want HEAD, 200, and no bytes", resp.Code, line)
		}
	})
}

// Cookies, credentials, and query strings can carry secrets, and the access log keeps them for as long as the logs
// live, so none of them may reach it.
func TestAccessLogLeavesOutHeadersAndQueryStrings(t *testing.T) {
	handler, logs := loggedSite(t, newCatalog())
	const secret = "s3cr3t-value"
	r := httptest.NewRequest(http.MethodGet, library+"?tab=rules&token="+secret, nil)
	r.Header.Set("Cookie", "session="+secret)
	r.Header.Set("Authorization", "Bearer "+secret)
	r.Header.Set("X-Amz-Security-Token", secret)
	r.Header.Set("User-Agent", "agent-"+secret)
	r.Header.Set("Referer", "https://example.com/?q="+secret)
	r.Header.Set("X-Forwarded-For", "203.0.113.9")

	_, line := accessLine(t, handler, logs, r)

	for _, leak := range []string{secret, "203.0.113.9", "tab=rules"} {
		if strings.Contains(logs.String(), leak) {
			t.Fatalf("the access log holds %q: %v", leak, line)
		}
	}
}

// panicking is a catalog whose home page panics with a runtime error, as a bug in a page's read would, and whose
// library pages panic with value.
type panicking struct {
	catalog
	value any
}

func (panicking) HomePage(context.Context) (views.HomePage, error) {
	var cards []views.LibraryCard
	return views.HomePage{Libraries: cards[:len(cards)+3]}, nil
}

func (p panicking) LibraryPage(context.Context, string, string) (views.LibraryPage, error) {
	panic(p.value)
}

// errorLines returns the error level lines in logs.
func errorLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var found []map[string]any
	for line := range strings.Lines(logs.String()) {
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatal(err)
		}
		if fields["level"] == "ERROR" {
			found = append(found, fields)
		}
	}
	return found
}

// A panic fails only its own request: it's logged once, with what finding the bug needs, and the visitor gets the
// page any other failure gets, which can't be cached.
func TestPagesRecoverFromAPanicWithTheUnavailablePage(t *testing.T) {
	handler, logs := loggedSite(t, panicking{catalog: newCatalog()})

	resp, access := accessLine(t, handler, logs, httptest.NewRequest(http.MethodGet, "/", nil))

	if resp.Code != http.StatusServiceUnavailable || resp.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("answered %d with Cache-Control %q", resp.Code, resp.Header().Get("Cache-Control"))
	}
	assertShows(t, resp.Body.String(), "Rulemart can't show this page right now.")
	if strings.Contains(resp.Body.String(), "out of range") {
		t.Fatal("the response exposes the panic")
	}
	if access["status"] != float64(http.StatusServiceUnavailable) || access["cache"] != "no-store" {
		t.Fatalf("the access log recorded %v", access)
	}
	logged := errorLines(t, logs)
	if len(logged) != 1 {
		t.Fatalf("logged %d error lines, want 1:\n%s", len(logged), logs)
	}
	panicLine := logged[0]
	stack, _ := panicLine["stack"].(string)
	if panicLine["msg"] != "panic" || panicLine["route"] != "/{$}" || panicLine["requestID"] != "request-123" ||
		!strings.HasPrefix(panicLine["panic"].(string), "runtime error: slice bounds out of range") ||
		!strings.Contains(stack, "panicking.HomePage") {
		t.Fatalf("logged %v", panicLine)
	}

	if next := get(t, handler, errorsRule); next.Code != http.StatusOK {
		t.Fatalf("the next request got %d", next.Code)
	}
}

// A panic's value can be anything, such as an error that holds a connection string, so only a runtime error's own
// text is logged, and any other value only by its type.
func TestPanicLogLeavesOutValuesThatAreNotRuntimeErrors(t *testing.T) {
	const secret = "postgres://rulemart:hunter2@db.example/rulemart"
	for name, value := range map[string]any{
		"a string": "could not connect to " + secret,
		"an error": fmt.Errorf("connect %s: refused", secret),
	} {
		t.Run(name, func(t *testing.T) {
			handler, logs := loggedSite(t, panicking{catalog: newCatalog(), value: value})

			resp, _ := accessLine(t, handler, logs, httptest.NewRequest(http.MethodGet, library, nil))

			if resp.Code != http.StatusServiceUnavailable {
				t.Fatalf("answered %d", resp.Code)
			}
			if strings.Contains(logs.String(), "hunter2") {
				t.Fatalf("the logs hold the panic's value:\n%s", logs)
			}
			if logged := errorLines(t, logs); len(logged) != 1 || logged[0]["panic"] != fmt.Sprintf("%T", value) {
				t.Fatalf("logged %v, want the value's type", logged)
			}
		})
	}
}

// failingPages fails every page read with an error that names what it read by the request's path, as the store's
// errors do.
type failingPages struct{ catalog }

func (failingPages) LibraryPage(_ context.Context, owner, name string) (views.LibraryPage, error) {
	return views.LibraryPage{}, fmt.Errorf("load library %s/%s: connection refused", owner, name)
}

func (failingPages) RulePage(_ context.Context, owner, name, rulePath string) (views.RulePage, error) {
	return views.RulePage{}, fmt.Errorf("load rule %s/%s/%s: connection refused", owner, name, rulePath)
}

// A failure's error names the library or rule it couldn't read, which the request's path chose, so the failure log
// replaces those names with the route's, as the access log does.
func TestFailureLogLeavesOutThePathTheErrorNames(t *testing.T) {
	for path, want := range map[string]string{
		"/alice@example.test/reset-token-s3cr3t":              "load library {owner}/{repo}: connection refused",
		"/alice@example.test/reset-token-s3cr3t/techs/go/x-y": "load rule {owner}/{repo}/{rule...}: connection refused",
	} {
		t.Run(path, func(t *testing.T) {
			handler, logs := loggedSite(t, failingPages{newCatalog()})

			resp, _ := accessLine(t, handler, logs, httptest.NewRequest(http.MethodGet, path, nil))

			if resp.Code != http.StatusServiceUnavailable {
				t.Fatalf("answered %d", resp.Code)
			}
			for _, leak := range []string{"alice", "reset-token", "s3cr3t", "techs/go"} {
				if strings.Contains(logs.String(), leak) {
					t.Fatalf("the logs hold %q:\n%s", leak, logs)
				}
			}
			if logged := errorLines(t, logs); len(logged) != 1 || logged[0]["error"] != want {
				t.Fatalf("logged %v, want the error %q", logged, want)
			}
		})
	}
}
