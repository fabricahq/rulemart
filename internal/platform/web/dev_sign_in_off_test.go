//go:build !rulemartdev

package web_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/platform/web"
)

// A release build has no dev sign-in: no route signs in a test user, and the sign-in page offers none.
func TestAReleaseBuildHasNoDevSignIn(t *testing.T) {
	site := newAccountsSite(t, nil)

	resp := send(t, site.handler, request{method: http.MethodPost, target: "/account/dev-sign-in?as=test_user"})
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin"}))

	if web.DevSignIn {
		t.Error("a build without the rulemartdev tag reports a dev sign-in")
	}
	if resp.StatusCode != http.StatusNotFound || cookie(resp, sessionCookie) != nil {
		t.Errorf("POST /account/dev-sign-in answered %d", resp.StatusCode)
	}
	if strings.Contains(page, "test_user") || strings.Contains(page, "Local build") {
		t.Error("the sign-in page offers a test user")
	}
}
