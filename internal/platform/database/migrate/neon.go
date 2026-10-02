// Deriving Neon's direct connection string, which migrations need, from the pooled one the functions use.

package migrate

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DirectConnString returns the direct connection string for Neon's pooled connection string pooled. Neon names a
// pooled endpoint by adding -pooler to the first label of the direct host, so the result differs only in its host.
// It rejects anything else, so a change to Neon's naming fails loudly instead of migrating through the pooler, and
// rejects a query that sends the connection to another host or port. Its errors never repeat the connection string.
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
	host := direct + "." + domain
	u.Host = host
	if port != "" {
		u.Host += ":" + port
	}
	directConnString := u.String()
	if err := requireDestination(directConnString, host, port); err != nil {
		return "", err
	}
	return directConnString, nil
}

// requireDestination returns an error unless every destination pgx would try for connString, including its TLS
// fallbacks, is host at port, or the default port when port is empty. pgx honors host and port in the query, which
// would otherwise override the rewritten host.
func requireDestination(connString, host, port string) error {
	config, err := pgx.ParseConfig(connString)
	if err != nil {
		// pgx's parse errors quote the connection string, credentials included, so leave them out.
		return errors.New("the pooled connection string isn't a valid Postgres connection string")
	}
	want := uint16(5432)
	if port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return errors.New("the pooled connection string has an invalid port")
		}
		want = uint16(n)
	}
	destinations := []*pgconn.FallbackConfig{{Host: config.Host, Port: config.Port}}
	destinations = append(destinations, config.Fallbacks...)
	for _, d := range destinations {
		if d.Host != host || d.Port != want {
			return errors.New("the pooled connection string must name one host, with no host or port override in its query")
		}
	}
	return nil
}
