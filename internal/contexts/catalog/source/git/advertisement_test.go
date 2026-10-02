package git_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
)

// pktLine encodes one line of Git's pkt-line format.
func pktLine(line string) string { return fmt.Sprintf("%04x%s", len(line)+4, line) }

// advertisingServer serves a smart HTTP reference advertisement: release/1, then branches more branches.
func advertisingServer(t *testing.T, branches int) *httptest.Server {
	t.Helper()
	hash := strings.Repeat("a", 40)
	var body strings.Builder
	body.WriteString(pktLine("# service=git-upload-pack\n") + "0000")
	body.WriteString(pktLine(hash + " refs/tags/release/1\x00multi_ack side-band-64k ofs-delta\n"))
	for i := range branches {
		body.WriteString(pktLine(fmt.Sprintf("%s refs/heads/branch-%07d\n", hash, i)))
	}
	body.WriteString("0000")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/info/refs") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		_, _ = w.Write([]byte(body.String()))
	}))
	t.Cleanup(server.Close)
	return server
}

// A repository can advertise any number of branches and other references beside its release tags, so the listing
// must stop reading the advertisement at its limit rather than hold all of it.
func TestListReleaseTagsStopsReadingAnAdvertisementPastItsLimit(t *testing.T) {
	bounded := limits
	bounded.RefsBytes = 64 << 10
	for name, tc := range map[string]struct {
		branches int
		want     string
	}{
		"within the limit": {10, ""},
		"past the limit":   {10_000, "more than 65536 bytes of references"},
	} {
		t.Run(name, func(t *testing.T) {
			server := advertisingServer(t, tc.branches)

			tags, err := git.ListReleaseTags(context.Background(), server.URL+"/example/rules.git", bounded)

			switch {
			case tc.want == "" && (err != nil || tags[1] != strings.Repeat("a", 40) || len(tags) != 1):
				t.Fatalf("listed %v, %v; want release/1", tags, err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("got %v, %v; want an error containing %q", tags, err, tc.want)
			}
		})
	}
}
