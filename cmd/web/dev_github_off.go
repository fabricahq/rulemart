//go:build !rulemartdev

// Release builds have no fake GitHub: dev_github.go, which a rulemartdev build compiles instead, holds it.

package main

import (
	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
)

// devGitHub is never called in a release build, whose web.DevSignIn is false.
func devGitHub(accountsapp.TokenKeys) (*github.API, *github.App, accountsapp.TokenKeys) {
	panic("a release build has no fake GitHub")
}
