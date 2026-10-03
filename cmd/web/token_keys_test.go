package main

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/secret"
)

// parameterStore is one SSM parameter, which an operator may replace.
type parameterStore struct {
	mu    sync.Mutex
	value string
	reads int
}

func (p *parameterStore) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(p.value)}}, nil
}

func (p *parameterStore) replace(value string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.value = value
}

// A warm instance that read the token key before an operator replaced it opens a token that an instance started since
// sealed under the new key: opening fails with the key it kept, so it reads the parameter again, once, and opens it.
func TestAWarmInstanceOpensATokenSealedUnderAReplacedKey(t *testing.T) {
	ctx := context.Background()
	key := func(b byte) string { return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(b), 32))) }
	parameter := &parameterStore{value: key('a')}
	_, connString := databasetest.New(t)
	store := postgres.New(databasetest.AsWebRole(t, connString))
	instance := func() accountsapp.Sessions {
		return accountsapp.Sessions{Store: store, TokenKeys: tokenKeys{secret: secret.FromParameter(parameter, "/rulemart/test/token-key")}}
	}
	warm := instance()
	if _, err := warm.TokenKeys.TokenKey(ctx); err != nil {
		t.Fatal(err)
	}

	parameter.replace(key('b'))
	_, session, err := instance().SignIn(ctx, domain.Identity{GitHubUserID: 583231, Login: "octocat"}, "gho_token", "")
	if err != nil {
		t.Fatal(err)
	}
	reads := parameter.reads

	got, err := warm.GitHubToken(ctx, session.Token)

	if err != nil || got != "gho_token" {
		t.Fatalf("the warm instance opened %q, %v", got, err)
	}
	if parameter.reads != reads+1 {
		t.Errorf("read the parameter %d times, want once", parameter.reads-reads)
	}
	if again, err := warm.GitHubToken(ctx, session.Token); err != nil || again != "gho_token" || parameter.reads != reads+1 {
		t.Errorf("opening again got %q, %v, and read the parameter %d times in all, want once", again, err, parameter.reads-reads)
	}
}
