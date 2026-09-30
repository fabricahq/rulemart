// Deriving Neon's direct connection string, which migrations need, from the pooled one the functions use.

package migrate

import (
	"errors"
	"net/url"
	"strings"
)

// DirectConnString returns the direct connection string for Neon's pooled connection string pooled. Neon names a
// pooled endpoint by adding -pooler to the first label of the direct host, so the result differs only in its host.
// It rejects anything else, so a change to Neon's naming fails loudly instead of migrating through the pooler.
func DirectConnString(pooled string) (string, error) {
	u, err := url.Parse(pooled)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
		// The parse error would repeat the connection string, so leave it out.
		return "", errors.New("the pooled connection string isn't a postgres:// URL")
	}
	endpoint, domain, found := strings.Cut(u.Hostname(), ".")
	direct, pooledEndpoint := strings.CutSuffix(endpoint, "-pooler")
	if !found || !pooledEndpoint || direct == "" {
		return "", errors.New("the connection string's host isn't a Neon pooler host (<endpoint>-pooler.<domain>)")
	}
	port := u.Port()
	u.Host = direct + "." + domain
	if port != "" {
		u.Host += ":" + port
	}
	return u.String(), nil
}
