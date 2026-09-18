package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// The hx-csp nonce gate reads the page nonce back from an enforcing Content-Security-Policy
// response header. Report-Only emits a different header name and disabled emits none, so in
// both cases the extension would strip every htmx attribute off swapped-in fragments.
func TestBase_CSPEnforced(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enabled    bool
		reportOnly bool
		want       bool
	}{
		{"enforcing", true, false, true},
		{"report only", true, true, false},
		{"disabled", false, false, false},
		{"disabled and report only", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.Cfg.HTTP.CSP.Enabled = tc.enabled
			f.Cfg.HTTP.CSP.ReportOnly = tc.reportOnly

			d := f.Deps.Base(httptest.NewRequest(http.MethodGet, "/", nil), "t")

			require.Equal(t, tc.want, d.CSPEnforced)
		})
	}
}
