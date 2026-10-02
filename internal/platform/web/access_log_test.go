package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

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
	handler, logs := loggedSite(t, newCatalog())
	for _, tc := range []struct {
		path, route string
		status      int
	}{
		{"/", "/{$}", http.StatusOK},
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
	for _, path := range []string{"missing", "nothing-here", "Example", "return-errors", "tab="} {
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
