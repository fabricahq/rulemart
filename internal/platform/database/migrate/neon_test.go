package migrate

import "testing"

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
