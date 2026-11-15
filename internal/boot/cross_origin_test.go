package boot_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/platform/authn"
)

func serveWithHeaders(t *testing.T, h http.Handler, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestConsoleRefusesCrossOriginPosts guards the console chain's CSRF check without touching the bearer surfaces.
func TestConsoleRefusesCrossOriginPosts(t *testing.T) {
	f := newMCPFixture(t, mcpOpts{enabled: true})
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range form {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	for name, headers := range map[string]map[string]string{
		"cross-site fetch metadata": {"Sec-Fetch-Site": "cross-site"},
		"same-site sibling host":    {"Sec-Fetch-Site": "same-site", "Origin": "https://evil.example.com"},
		"foreign Origin only":       {"Origin": "https://evil.example.com"},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			rec := serveWithHeaders(t, f.srv.Web, "/logout", "", with(headers))
			require.Equal(t, http.StatusForbidden, rec.Code, "a cross-origin console POST got through; body=%s", rec.Body.String())
		})
	}

	for name, headers := range map[string]map[string]string{
		"same-origin fetch metadata": {"Sec-Fetch-Site": "same-origin"},
		"Origin matching Host":       {"Origin": "http://example.com"},
		"Origin matching baseURL":    {"Origin": "http://127.0.0.1"},
		"no browser headers":         {},
	} {
		t.Run("accepts "+name, func(t *testing.T) {
			rec := serveWithHeaders(t, f.srv.Web, "/logout", "", with(headers))
			require.NotEqual(t, http.StatusForbidden, rec.Code, "a same-origin console POST was refused; body=%s", rec.Body.String())
		})
	}

	crossSite := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example.com"}

	t.Run("leaves /api bearer calls alone", func(t *testing.T) {
		read := f.issuer.mint(t, controlAudience, []string{authn.ScopePostsRead})
		headers := map[string]string{
			"Content-Type":             "application/json",
			"Connect-Protocol-Version": "1",
			"Authorization":            "Bearer " + read,
		}
		for k, v := range crossSite {
			headers[k] = v
		}
		rec := serveWithHeaders(t, f.srv.Web, todoListProcedure, `{"projectId":"`+f.projectID+`"}`, headers)
		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	})

	t.Run("leaves /mcp bearer calls alone", func(t *testing.T) {
		headers := map[string]string{
			"Content-Type":  "application/json",
			"Accept":        mcpProtocolHeader,
			"Authorization": "Bearer " + f.readKey,
		}
		for k, v := range crossSite {
			headers[k] = v
		}
		rec := serveWithHeaders(t, f.srv.Web, mcpPath, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, headers)
		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	})
}
