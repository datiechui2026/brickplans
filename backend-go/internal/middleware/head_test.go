package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHeadToGetMirrorsGet pins the contract the SEO audit depends on: a HEAD
// response must carry the same status as GET, with no body and a matching
// Content-Length. Before this wrapper every HEAD landed in NoRoute and the
// bodyless 200 that came out is the soft-404 signature crawlers penalise.
func TestHeadToGetMirrorsGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("hello page"))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope"))
	})
	h := HeadToGet(mux)

	cases := []struct {
		path string
		code int
	}{
		{"/ok", http.StatusOK},
		{"/moved", http.StatusMovedPermanently},
		{"/gone", http.StatusNotFound},
		{"/never-registered", http.StatusNotFound}, // ServeMux's own 404 must survive too
	}
	for _, tc := range cases {
		get := httptest.NewRecorder()
		h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if get.Code != tc.code {
			t.Fatalf("GET %s = %d, want %d", tc.path, get.Code, tc.code)
		}

		head := httptest.NewRecorder()
		h.ServeHTTP(head, httptest.NewRequest(http.MethodHead, tc.path, nil))
		if head.Code != tc.code {
			t.Errorf("HEAD %s = %d, want %d (same as GET)", tc.path, head.Code, tc.code)
		}
		if head.Body.Len() != 0 {
			t.Errorf("HEAD %s returned a body (%q)", tc.path, head.Body.String())
		}
		if tc.code != http.StatusNotFound && tc.code != http.StatusMovedPermanently {
			if got, want := head.Header().Get("Content-Length"), itoa(get.Body.Len()); got != want {
				t.Errorf("HEAD %s Content-Length = %q, want %q", tc.path, got, want)
			}
		}
	}

	// GET must still be untouched (body, headers and status all pass through).
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/gone", nil))
	if get.Body.String() != "nope" || get.Code != http.StatusNotFound {
		t.Errorf("GET /gone = %d %q, want 404 %q", get.Code, get.Body.String(), "nope")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
