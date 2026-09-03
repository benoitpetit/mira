package rest_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/interfaces/rest"
)

func TestServeDashboardServesExplorerAndAssets(t *testing.T) {
	mux := http.NewServeMux()
	rest.ServeDashboard(mux)

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/", want: "searchKind"},
		{path: "/app.js", want: "loadStats"},
		{path: "/styles.css", want: "--accent"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", tc.path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("GET %s did not contain %q", tc.path, tc.want)
		}
	}
}
