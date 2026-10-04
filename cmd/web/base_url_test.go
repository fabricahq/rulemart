package main

import (
	"testing"
)

// env returns a getenv that reads values, and nothing else.
func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// The base URL RULEMART_BASE_URL names is the one pages use, in any build, wherever the server listens.
func TestBaseURLIsTheOneRULEMART_BASE_URLNames(t *testing.T) {
	got, err := newBaseURL(env(map[string]string{"RULEMART_BASE_URL": "https://rulemart.example", "ADDR": "127.0.0.1:9000"}))

	if err != nil || got == nil || got.String() != "https://rulemart.example" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestBaseURLRefusesOneThatIsntAnOrigin(t *testing.T) {
	if got, err := newBaseURL(env(map[string]string{"RULEMART_BASE_URL": "https://rulemart.example/catalog"})); err == nil {
		t.Fatalf("accepted %v", got)
	}
}
