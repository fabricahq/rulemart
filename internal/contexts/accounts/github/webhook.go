// Check and read the deliveries of the GitHub App's webhook: GitHub signs each with the webhook's secret, and Rulemart
// acts on the ones that say an installation was removed, suspended, or unsuspended, or that the repositories it reads
// changed, by asking GitHub what the installation's state is now.

package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// WebhookInstallation returns the ID of the installation a delivery of the app's webhook says changed, after checking
// that signature, its X-Hub-Signature-256 header, is the HMAC-SHA256 of body with the webhook's secret. event is its
// X-GitHub-Event header. It fails with domain.ErrBadSignature for a delivery GitHub didn't sign, and
// domain.ErrIgnoredEvent for one that changes nothing Rulemart keeps.
func (a *App) WebhookInstallation(ctx context.Context, event string, body []byte, signature string) (int64, error) {
	secret, err := a.config.WebhookSecret.Value(ctx)
	if err != nil {
		return 0, fmt.Errorf("read the webhook's secret: %v", err)
	}
	if !ValidSignature([]byte(secret), body, signature) {
		return 0, domain.ErrBadSignature
	}
	var delivery struct {
		Action       string `json:"action"`
		Installation struct {
			ID    int64 `json:"id"`
			AppID int64 `json:"app_id"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &delivery); err != nil {
		return 0, fmt.Errorf("decode the delivery: %v", err)
	}
	id := delivery.Installation.ID
	switch {
	case id <= 0 || (delivery.Installation.AppID != 0 && delivery.Installation.AppID != a.config.ID):
		return 0, domain.ErrIgnoredEvent
	case event == "installation" && slices.Contains([]string{"deleted", "suspend", "unsuspend", "new_permissions_accepted"}, delivery.Action),
		event == "installation_repositories":
		return id, nil
	}
	return 0, domain.ErrIgnoredEvent
}

// ValidSignature reports whether signature, as GitHub's X-Hub-Signature-256 header writes it, sha256= and the hex
// HMAC-SHA256 of body with secret, is body's, compared in constant time.
func ValidSignature(secret, body []byte, signature string) bool {
	given, ok := strings.CutPrefix(signature, "sha256=")
	if !ok {
		return false
	}
	sum, err := hex.DecodeString(given)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(sum, mac.Sum(nil))
}
