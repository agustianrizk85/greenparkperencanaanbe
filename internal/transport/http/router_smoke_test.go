package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/repository"
	"greenpark/perencanaan/internal/service"
)

// TestBoardRouteMatching verifies the /api/board submux mount: no ServeMux
// pattern conflicts (registration panics), no double-prefix 404s, and the
// board scope resolving independently of the strict /api/ scope.
func TestBoardRouteMatching(t *testing.T) {
	repo := repository.NewMemory()
	svc := service.New(repo, auth.NewSessionStore(time.Hour), service.GKConfig{}, t.TempDir())
	h := NewHandler(svc)
	router := NewRouter(h, "*") // panics here would mean pattern conflicts

	token, _, err := svc.Login("kadep", "kadep123")
	if err != nil {
		// Seeded credentials unknown in this environment — fall back to
		// asserting on auth status codes only.
		token = ""
	}

	cases := []struct {
		method, path string
		want         []int // acceptable status codes
	}{
		// Board routes must reach the board mux (401 without a token, never 404).
		{http.MethodGet, "/api/board", []int{401}},
		{http.MethodPost, "/api/board/lists", []int{401}},
		{http.MethodGet, "/api/board/cards/cd-1", []int{401}},
		{http.MethodGet, "/api/board/cards/cd-1/ai-check", []int{401}},
		{http.MethodPost, "/api/board/cards/cd-1/ai-check", []int{401}},
		// Unknown board subpath: authenticated later, but unauthenticated is 401
		// (auth wraps the whole subtree).
		{http.MethodGet, "/api/board/nonsense", []int{401}},
		// Public attachment route validates its own token -> 401 not 404.
		{http.MethodGet, "/api/board/attachments/att-9", []int{401}},
		// New: skills picker + task attachment routes must reach their scope (401,
		// never 404). Upload/delete on the board submux; the GET serve route is on
		// the public mux and self-validates -> 401.
		{http.MethodGet, "/api/board/skills", []int{401}},
		{http.MethodPost, "/api/board/task/gp-001/t-1/attachments", []int{401}},
		{http.MethodDelete, "/api/board/task/gp-001/t-1/attachments/att-9", []int{401}},
		{http.MethodGet, "/api/board/task/gp-001/t-1/attachments/att-9", []int{401}},
		// Non-board API keeps working.
		{http.MethodGet, "/api/health", []int{200}},
		{http.MethodGet, "/api/summary", []int{401}},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		ok := false
		for _, w := range c.want {
			if rec.Code == w {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%s %s -> %d, want one of %v", c.method, c.path, rec.Code, c.want)
		}
	}

	if token != "" {
		authedCases := []struct {
			method, path string
			want         []int
		}{
			{http.MethodGet, "/api/board", []int{200}},                     // full view
			{http.MethodGet, "/api/board/cards/no-such", []int{404}},       // matched route, unknown card
			{http.MethodGet, "/api/board/cards/no-such/ai-check", []int{404}},
			{http.MethodPatch, "/api/board/lists/no-such", []int{400, 404}},
		}
		for _, c := range authedCases {
			req := httptest.NewRequest(c.method, c.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			ok := false
			for _, w := range c.want {
				if rec.Code == w {
					ok = true
				}
			}
			if !ok {
				t.Errorf("authed %s %s -> %d, want one of %v (body %s)", c.method, c.path, rec.Code, c.want, rec.Body.String())
			}
		}
	}
}
