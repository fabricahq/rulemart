package secret

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// parameter is an SSM parameter whose value a test sets, counting reads.
type parameter struct {
	value string
	reads int
}

func (p *parameter) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	p.reads++
	if !aws.ToBool(in.WithDecryption) {
		panic("read a SecureString without decrypting it")
	}
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(p.value)}}, nil
}

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestFromEnvReadsTheValueOrNothing(t *testing.T) {
	ctx := context.Background()
	s, err := FromEnv(ctx, env(map[string]string{"GITHUB_CLIENT_SECRET": "local-secret"}), "GITHUB_CLIENT_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := s.Value(ctx); err != nil || value != "local-secret" {
		t.Errorf("got %q, %v", value, err)
	}
	s.Forget()
	if value, _ := s.Value(ctx); value != "local-secret" {
		t.Errorf("a value given directly was forgotten: got %q", value)
	}

	if s, err := FromEnv(ctx, env(nil), "GITHUB_CLIENT_SECRET"); s != nil || err != nil {
		t.Errorf("with neither variable: got %v, %v; want nil", s, err)
	}
	both := map[string]string{"GITHUB_CLIENT_SECRET": "local-secret", "GITHUB_CLIENT_SECRET_PARAMETER": "/rulemart/x"}
	if _, err := FromEnv(ctx, env(both), "GITHUB_CLIENT_SECRET"); err == nil || strings.Contains(err.Error(), "local-secret") {
		t.Errorf("with both variables: got %v, want an error without the value", err)
	}
}

func TestAParameterIsReadOnceUntilForgotten(t *testing.T) {
	ctx := context.Background()
	p := &parameter{value: "first"}
	s := FromParameter(p, "/rulemart/test/secret")
	for range 3 {
		if value, err := s.Value(ctx); err != nil || value != "first" {
			t.Fatalf("got %q, %v", value, err)
		}
	}
	if p.reads != 1 {
		t.Errorf("read the parameter %d times, want once", p.reads)
	}
	p.value = "rotated"
	s.Forget()
	if value, _ := s.Value(ctx); value != "rotated" || p.reads != 2 {
		t.Errorf("after Forget got %q with %d reads, want the rotated value read again", value, p.reads)
	}
}

// infra-catalog's parameter module creates a parameter holding a placeholder until an operator sets it, so the secret
// isn't there yet, and the next use reads it again.
func TestAParameterWithoutASecretYetFailsAndIsReadAgain(t *testing.T) {
	ctx := context.Background()
	for _, value := range []string{"", "REPLACE_IN_AWS_CONSOLE"} {
		p := &parameter{value: value}
		s := FromParameter(p, "/rulemart/test/secret")
		if _, err := s.Value(ctx); err == nil || !strings.Contains(err.Error(), "/rulemart/test/secret") {
			t.Errorf("with %q: got %v, want an error naming the parameter", value, err)
		}
		p.value = "set"
		if got, err := s.Value(ctx); err != nil || got != "set" {
			t.Errorf("once set: got %q, %v", got, err)
		}
	}
}
