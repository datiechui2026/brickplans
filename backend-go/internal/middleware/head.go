package middleware

import (
	"net/http"
	"strconv"
)

// HeadToGet turns HEAD into GET *before* the router sees the request, and drops
// the response body afterwards while keeping the real status and a matching
// Content-Length.
//
// Why this wraps an http.Handler instead of being a gin middleware (it was
// written as `r.Use(middleware.HeadToGet())` until 2026-10-24, and that never
// worked): gin resolves the route through its per-method trees *before* running
// any handler in the chain, and it only builds trees for methods that were
// registered. So a HEAD request fell straight into NoRoute on every single page,
// and the gin-style middleware — which ran after the miss — could not undo it.
// Rewriting the method at the http.Handler boundary is the one place that is
// genuinely "before routing" for any router.
//
// Measured consequences of the old behaviour (2026-10-24, production):
//   - HEAD on any real page: router answered 404, body was discarded before the
//     status was flushed, so the wire saw an implicit 200 — for everything.
//   - HEAD on an unknown URL: same 200. That is the soft-404 signature Google
//     uses to spend crawl budget on dead URLs, and it is why `curl -sI` reported
//     200 for pages that GET served as 404.
//
// After this wrapper, HEAD returns exactly the status GET would.
func HeadToGet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		// Clone so nothing holding the original request is surprised; only the
		// method changes, the URL/headers/body stay intact.
		req := r.Clone(r.Context())
		req.Method = http.MethodGet

		hw := &headWriter{ResponseWriter: w}
		next.ServeHTTP(hw, req)
		hw.finish()
	})
}

// headWriter buffers the status and counts the body instead of forwarding it, so
// HEAD can answer with the same headers GET would (notably Content-Length).
type headWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	size        int64
}

func (w *headWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status, w.wroteHeader = code, true
}

func (w *headWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.size += int64(len(b))
	return len(b), nil // discard the body, pretend the write succeeded
}

// finish flushes the buffered status plus a Content-Length equal to the body GET
// would have sent (RFC 9110 §9.3.2: HEAD carries the same headers, no body).
func (w *headWriter) finish() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	switch w.status {
	case http.StatusNoContent, http.StatusNotModified:
		// These must not carry a body nor a Content-Length.
		w.Header().Del("Content-Length")
	default:
		w.Header().Set("Content-Length", strconv.FormatInt(w.size, 10))
	}
	w.ResponseWriter.WriteHeader(w.status)
}
