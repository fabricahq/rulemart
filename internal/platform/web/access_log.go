// Log one line for every request the handler serves.

package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/platform/logging"
)

// logRequests logs one line for each request next serves: the route pattern it matched, the method, the status and
// body bytes sent, how long it took, the Cache-Control it sent, and the request ID. It leaves out the path, query
// string, and headers, which can identify visitors or carry secrets; CloudFront's own logs keep what analysis needs
// of those.
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		s.Log.InfoContext(r.Context(), "request", "route", s.route(r), "method", r.Method, "status", recorder.statusCode(),
			"bytes", recorder.bytes, "duration_ms", logging.Milliseconds(time.Since(start)),
			"cache", w.Header().Get("Cache-Control"), "requestID", s.requestID(r))
	})
}

// unmatchedRoute is the route logged for a request that matched no registered pattern, such as one the mux
// redirected to a cleaner path.
const unmatchedRoute = "unmatched"

// route returns the pattern r matched, without its method, such as /{owner}/{repo}, or unmatchedRoute. The mux
// records the pattern on r as it routes it; for its own redirects it may record a path instead, so route accepts
// only the patterns New registered.
func (s *server) route(r *http.Request) string {
	if !s.routes[r.Pattern] {
		return unmatchedRoute
	}
	_, path, found := strings.Cut(r.Pattern, " ")
	if !found {
		return r.Pattern
	}
	return path
}

// requestID returns r's ID from Options.RequestID, or "" without one.
func (s *server) requestID(r *http.Request) string {
	if s.RequestID == nil {
		return ""
	}
	return s.RequestID(r)
}

// responseRecorder passes a response through, recording its status and how many body bytes were written.
type responseRecorder struct {
	http.ResponseWriter
	// status is 0 until the handler writes the header or the body.
	status int
	bytes  int
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// statusCode returns the status sent, which is 200 when the handler wrote nothing.
func (w *responseRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
