package migrate

import (
	"strings"
	"testing"
)

func TestDirectConnStringDropsPoolerFromTheEndpoint(t *testing.T) {
	for name, tc := range map[string]struct{ pooled, direct string }{
		"typical": {
			"postgresql://app:secret@ep-divine-bread-ar7rtkac-pooler.c-4.us-west-2.aws.neon.tech/neondb?sslmode=require&channel_binding=require",
			"postgresql://app:secret@ep-divine-bread-ar7rtkac.c-4.us-west-2.aws.neon.tech/neondb?sslmode=require&channel_binding=require",
		},
		"postgres scheme with port": {
			"postgres://app:secret@ep-a-pooler.example.neon.tech:5432/db",
			"postgres://app:secret@ep-a.example.neon.tech:5432/db",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DirectConnString(tc.pooled)
			if err != nil || got != tc.direct {
				t.Fatalf("got %q, %v; want %q", got, err, tc.direct)
			}
		})
	}
}

// Anything but a Neon pooled URL is rejected, so the migration never runs through the pooler or at the wrong host.
func TestDirectConnStringRejectsWhatIsNotAPooledNeonURL(t *testing.T) {
	for name, pooled := range map[string]string{
		"direct host":                 "postgresql://app:secret@ep-a.example.neon.tech/db",
		"pooler outside the endpoint": "postgresql://app:secret@ep-a.example-pooler.neon.tech/db",
		"only the pooler suffix":      "postgresql://app:secret@-pooler.example.neon.tech/db",
		"single-label host":           "postgresql://app:secret@ep-a-pooler/db",
		"key-value format":            "host=ep-a-pooler.example.neon.tech user=app",
		"other scheme":                "mysql://app:secret@ep-a-pooler.example.neon.tech/db",
		"empty":                       "",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := DirectConnString(pooled); err == nil {
				t.Fatalf("accepted %q as %q", pooled, got)
			}
		})
	}
}

// pgx's parse errors quote the connection string, so a malformed value used to reach public CI logs with password
// fragments in it.
func TestDirectConnStringKeepsCredentialsOutOfItsErrors(t *testing.T) {
	_, err := DirectConnString("postgresql://app:secret@ep-a-pooler.example.neon.tech/db?password=SECRETFRAGMENT&MORESECRET")
	if err == nil {
		t.Fatal("accepted a connection string pgx can't parse")
	}
	for _, leak := range []string{"SECRETFRAGMENT", "MORESECRET", "secret"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error %q repeats %q from the connection string", err, leak)
		}
	}
}

// pgx honors host and port in the query, which used to override the rewritten host, so migrations could still reach
// the pooler or anywhere else.
func TestDirectConnStringRejectsDestinationOverrides(t *testing.T) {
	for name, pooled := range map[string]string{
		"host override":  "postgresql://app:secret@ep-a-pooler.example.neon.tech/db?host=127.0.0.1&port=1",
		"pooler in host": "postgresql://app:secret@ep-a-pooler.example.neon.tech/db?host=ep-a-pooler.example.neon.tech",
		"several hosts":  "postgresql://app:secret@ep-a-pooler.example.neon.tech,ep-b-pooler.example.neon.tech/db",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := DirectConnString(pooled); err == nil {
				t.Fatalf("accepted %q as %q", pooled, got)
			}
		})
	}
}
