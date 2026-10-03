// Check and read the deliveries of the GitHub App's webhook: GitHub signs each with the webhook's secret, and Rulemart
// acts on the ones that say an installation was removed, or that the repositories it reads changed.

package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// WebhookChange returns the change to an installation a delivery of the app's webhook reports, after checking that
// signature, its X-Hub-Signature-256 header, is the HMAC-SHA256 of body with the webhook's secret. event is its
// X-GitHub-Event header. It fails with domain.ErrBadSignature for a delivery GitHub didn't sign, and domain.ErrIgnoredEvent for one
// that changes nothing Rulemart keeps.
func (a *App) WebhookChange(ctx context.Context, event string, body []byte, signature string) (domain.InstallationChange, error) {
	secret, err := a.config.WebhookSecret.Value(ctx)
	if err != nil {
		return domain.InstallationChange{}, fmt.Errorf("read the webhook's secret: %v", err)
	}
	if !ValidSignature([]byte(secret), body, signature) {
		return domain.InstallationChange{}, domain.ErrBadSignature
	}
	var delivery struct {
		Action       string `json:"action"`
		Installation struct {
			ID    int64 `json:"id"`
			AppID int64 `json:"app_id"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &delivery); err != nil {
		return domain.InstallationChange{}, fmt.Errorf("decode the delivery: %v", err)
	}
	change := domain.InstallationChange{ID: delivery.Installation.ID}
	switch {
	case change.ID <= 0 || (delivery.Installation.AppID != 0 && delivery.Installation.AppID != a.config.ID):
		return domain.InstallationChange{}, domain.ErrIgnoredEvent
	case event == "installation" && (delivery.Action == "deleted" || delivery.Action == "suspend"):
		change.Removed = true
	case event == "installation" && (delivery.Action == "unsuspend" || delivery.Action == "new_permissions_accepted"),
		event == "installation_repositories":
	default:
		return domain.InstallationChange{}, domain.ErrIgnoredEvent
	}
	return change, nil
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
