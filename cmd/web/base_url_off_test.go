//go:build !rulemartdev

package main

import "testing"

// A release build names no base URL it wasn't given, so its pages never name a local address as canonical.
func TestReleaseBuildHasNoBaseURLUnlessConfigured(t *testing.T) {
	if got, err := newBaseURL(env(map[string]string{"ADDR": "127.0.0.1:8080"})); err != nil || got != nil {
		t.Fatalf("got %v, %v, want none", got, err)
	}
}
