// Read Code Rules' canonical group list, which ships with each release.

package catalog

import (
	_ "embed"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// canonicalGroupsYAML is canonical-groups.yaml from fabricahq/code-rules at v0.2.0, commit
// 6a63c7173bb5ae8b37cf22f30b8a4aede1d6b435, copied unchanged. Code Rules owns the list, so update it only by
// copying the file from a newer commit, and update the checksum its test pins with it.
//
//go:embed canonical-groups.yaml
var canonicalGroupsYAML []byte

// CanonicalGroups returns Code Rules' canonical group list, as the vendored parser reads it.
func CanonicalGroups() (domain.CanonicalGroups, error) {
	list, err := coderules.ParseCanonicalGroups(canonicalGroupsYAML, "canonical-groups.yaml")
	if err != nil {
		return domain.CanonicalGroups{}, fmt.Errorf("read canonical groups: %v", err)
	}
	return domain.NewCanonicalGroups(list), nil
}
