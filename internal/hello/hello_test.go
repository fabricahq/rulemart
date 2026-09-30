package hello

import (
	"strings"
	"testing"
	"time"
)

func TestValidateRejectsMessagesTheWebFunctionCouldNotSend(t *testing.T) {
	sent := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	valid := Message{Text: "hi", Source: "web", SentAt: sent}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid message rejected: %v", err)
	}
	for name, m := range map[string]Message{
		"empty body":    {},
		"no text":       {Source: "web", SentAt: sent},
		"no source":     {Text: "hi", SentAt: sent},
		"no sent time":  {Text: "hi", Source: "web"},
		"text too long": {Text: strings.Repeat("é", MaxTextLength+1), Source: "web", SentAt: sent},
		// Postgres text can't hold NUL or invalid UTF-8, so these could never be stored.
		"NUL in text":          {Text: "hello\x00world", Source: "web", SentAt: sent},
		"NUL in source":        {Text: "hi", Source: "we\x00b", SentAt: sent},
		"invalid UTF-8 text":   {Text: "caf\xe9", Source: "web", SentAt: sent},
		"invalid UTF-8 source": {Text: "hi", Source: "\xff", SentAt: sent},
	} {
		if err := m.Validate(); err == nil {
			t.Errorf("%s: accepted %+v", name, m)
		}
	}
}

func TestValidateCountsCharactersNotBytes(t *testing.T) {
	m := Message{Text: strings.Repeat("é", MaxTextLength), Source: "web", SentAt: time.Now()}
	if err := m.Validate(); err != nil {
		t.Fatalf("%d two-byte characters rejected: %v", MaxTextLength, err)
	}
}
