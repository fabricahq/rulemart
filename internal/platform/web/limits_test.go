package web_test

import (
	"testing"

	accountsdomain "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// The dashboard's read of a configuration takes as much as ingestion reads of a library's manifest, so the picker
// finds every library the catalog would take. The contexts don't import each other, so the server that composes
// them checks it.
func TestTheDashboardReadsAsLargeAManifestAsIngestion(t *testing.T) {
	if got, want := int64(accountsdomain.MaxConfigBytes), domain.DefaultLimits.Content.FileBytes; got != want {
		t.Errorf("the dashboard reads configurations of at most %d bytes, ingestion manifests of %d", got, want)
	}
}
