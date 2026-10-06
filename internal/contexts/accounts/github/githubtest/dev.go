// The fake GitHub a local build serves, so its test users have a dashboard to see.

package githubtest

import (
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/lib/githubapp/githubapptest"
)

// DevToken returns the GitHub token a local build's test user login signs in with, which DevFake knows.
func DevToken(login string) string { return "gho_dev_" + login }

// DevUsers returns who a local build's dev sign-in can sign in as, the users DevFake knows. Their IDs are far past
// GitHub's, their logins have an underscore, which no personal GitHub account's can, and they have no avatar, so they
// show their initial.
func DevUsers() []domain.Identity {
	return []domain.Identity{{GitHubUserID: devUserID, Login: devUser}, {GitHubUserID: devUser2ID, Login: devUser2}}
}

// The test users' logins and IDs, which DevUsers returns.
const (
	devUser    = "test_user"
	devUserID  = 9_000_000_001
	devUser2   = "test_user_2"
	devUser2ID = 9_000_000_002
	// DevInstallation is the installation of the GitHub App on test_user's account, which reads its private
	// repositories, and which DevFake's install page installs.
	DevInstallation = 1
)

// DevFake returns a fake GitHub for a local build: test_user belongs to the fabricahq organization, which publishes the
// two libraries a local catalog holds, and a third that test_user may only read, and test_user has two projects that import them, with rule versions behind
// them, and, once the GitHub App is installed, a private library and a private project. test_user_2 has nothing. The
// app's install page sends the visitor back to installedURL, as GitHub sends them to the app's setup URL.
func DevFake(installedURL string) *Fake {
	day := func(n int) time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC).AddDate(0, 0, -n) }
	// Its public libraries are ones real GitHub has, since the worker that checks a library added at /me/add reads real
	// GitHub, not this fake, except the one test_user may only read, which they can't add.
	manifest := "schemaVersion: 1\nname: Experimental rules\n"
	publicRules := "https://github.com/fabricahq/public-rules.git"
	testLibrary := "https://github.com/fabricahq/code-rules-test-library.git"
	return &Fake{
		Users: []User{
			{Token: DevToken(devUser), ID: devUserID, Login: devUser, Organizations: []Membership{{Organization: "fabricahq", Role: "member"}}},
			{Token: DevToken(devUser2), ID: devUser2ID, Login: devUser2},
		},
		Repositories: []Repository{
			{
				Owner: devUser, Name: "api-server", PushedAt: day(2),
				Files: map[string]string{domain.ProvenancePath: Provenance(
					ProvenanceSource{Name: "fabrica", Repository: publicRules, Release: 1, Groups: []string{"techs/go", "practices/testing"},
						Rules: map[string]string{"techs/go/errors-include-useful-diagnostic-data": "1.0.0", "practices/testing/keep-tests-independent": "1.0.0"}},
					ProvenanceSource{Name: "test-library", Repository: testLibrary, Release: 3, Groups: []string{"techs/go"},
						Rules: map[string]string{"techs/go/return-errors": "1.0.0", "techs/go/use-contexts": "1.2.1", "techs/go/name-tests": "1.0.0"}},
				)},
			},
			{
				Owner: devUser, Name: "site", PushedAt: day(4),
				Files: map[string]string{domain.ProvenancePath: Provenance(ProvenanceSource{Name: "fabrica", Repository: publicRules, Release: 1,
					Groups: []string{"practices/testing"}, Rules: map[string]string{"practices/testing/keep-tests-independent": "1.0.0"}})},
			},
			{Owner: devUser, Name: "notes", PushedAt: day(9), Files: map[string]string{"README.md": "Notes"}},
			{Owner: "fabricahq", Name: "public-rules", PushedAt: day(3), Files: map[string]string{"rule-library.yaml": manifest}, Tags: []string{"release/1"}},
			{
				Owner: "fabricahq", Name: "code-rules-test-library", PushedAt: day(20), Files: map[string]string{"rule-library.yaml": manifest},
				Tags: []string{"release/1", "release/2", "release/3", "release/4", "release/5", "release/6"},
			},
			// test_user belongs to fabricahq but may only read this one, so the picker shows a library they can't add.
			{Owner: "fabricahq", Name: "partner-rules", PushedAt: day(8), ReadOnly: true, Files: map[string]string{"rule-library.yaml": manifest}, Tags: []string{"release/2"}},
			{Owner: devUser, Name: "team-rules", Private: true, PushedAt: day(5), Files: map[string]string{"rule-library.yaml": manifest}, Tags: []string{"release/2"}},
			{
				Owner: devUser, Name: "billing-service", Private: true, PushedAt: day(6),
				Files: map[string]string{domain.ProvenancePath: Provenance(ProvenanceSource{Name: "testing", Repository: testLibrary, Release: 5,
					Groups: []string{"practices/testing"}, Rules: map[string]string{"practices/testing/verify-retry-limits": "2.1.0"}})},
			},
		},
		Installations: []Installation{
			{ID: DevInstallation, Account: devUser, AccountID: devUserID, Repositories: []string{devUser + "/team-rules", devUser + "/billing-service"}},
		},
		AppClientID:  "Iv1.devfake",
		AppKey:       githubapptest.NewKey(),
		AppSlug:      "rulemart-by-fabrica",
		InstalledURL: installedURL,
		InstallAs:    DevInstallation,
	}
}
