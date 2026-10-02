// Keep what depends on who's asking out of shared caches, and refuse writes that other sites start.

package web

import (
	"net/http"
)

// withPrivateResponses keeps every response that depends on who's asking out of shared caches. A request that
// carries a session cookie gets a page for its visitor, so its response, whatever it is, can't be cached; nor can a
// response that sets a cookie, since a cache would hand the cookie to everyone. Static files, and only responses the
// static files' route serves, are the same for everyone and stay cacheable: a missing page under /_static/ is a page
// like any other.
//
// It also marks every other response as varying with the Cookie header, so a browser that signs in doesn't show a
// page it kept from before. CloudFront passes Vary: Cookie through to browsers, and keeps signed-in requests apart by
// putting the session cookie in its cache key, but never caches their responses either way.
func withPrivateResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&privateWriter{ResponseWriter: w, request: r, signedIn: hasCookie(r, sessionCookie)}, r)
	})
}

// privateWriter sets a response's caching as withPrivateResponses describes, just before its header is written.
type privateWriter struct {
	http.ResponseWriter
	// request is the request answered, whose route the mux has recorded by the time the header is written.
	request *http.Request
	// signedIn is true when the request carries a session cookie, valid or not.
	signedIn bool
	wrote    bool
}

func (w *privateWriter) WriteHeader(status int) {
	if !w.wrote && w.request.Pattern != staticPattern {
		header := w.Header()
		header.Add("Vary", "Cookie")
		if w.signedIn || len(header.Values("Set-Cookie")) > 0 {
			header.Set("Cache-Control", privateCache)
		}
	}
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *privateWriter) Write(body []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *privateWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// withSameOriginWrites refuses a state-changing request, such as POST, that another site started, so no page
// elsewhere can sign a visitor out or act as them: browsers say where a request came from in Sec-Fetch-Site, or in
// Origin, which must then be this site's, or BaseURL. A request that names neither, such as from curl, isn't a
// browser's, and so carries no visitor's cookies against their will. The session cookie's SameSite=Lax is a second
// barrier. Writes carry no token in their bodies, because CloudFront can't forward a body a browser didn't sign to
// the web function; _internal/slices/5-sign-in.md explains.
func (s *server) withSameOriginWrites(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	if s.BaseURL != nil {
		// checkBaseURL made it a bare origin, which is what AddTrustedOrigin takes.
		_ = protection.AddTrustedOrigin(s.BaseURL.String())
	}
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Log.WarnContext(r.Context(), "cross-origin request refused", "method", r.Method, "requestID", s.requestID(r))
		// The refusal has every page's header, with the visitor's account slot, so the header doesn't move.
		r, ok := s.visit(w, r)
		if ok {
			s.renderPrivate(w, r, http.StatusForbidden, messagePage(s.chrome, "Refused", "Rulemart refused this request because another site sent it."))
		}
	}))
	return protection.Handler(next)
}
