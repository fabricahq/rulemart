// Bound the reference advertisement a remote sends before anything else: go-git reads all of it into memory before
// ingestion can count the release tags in it.

package git

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// go-git chooses a transport by URL scheme from one registry, so Fetch and ListReleaseTags reach remotes over HTTP and
// HTTPS through boundedAdvertisements. Nothing else in Rulemart uses go-git.
func init() {
	transport := githttp.NewClient(&http.Client{Transport: boundedAdvertisements{base: http.DefaultTransport}})
	client.InstallProtocol("https", transport)
	client.InstallProtocol("http", transport)
}

// refsLimitKey is the context key under which a listing or fetch passes its limits.RefsBytes to the transport.
type refsLimitKey struct{}

// withRefsLimit returns ctx carrying limit, the most bytes of references a remote may advertise to calls made with it.
func withRefsLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, refsLimitKey{}, limit)
}

// boundedAdvertisements is an HTTP transport that refuses a reference advertisement, the response to a smart HTTP
// .../info/refs request, larger than the limit its request's context carries. Other responses pass through.
type boundedAdvertisements struct {
	base http.RoundTripper
}

func (t boundedAdvertisements) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	limit, ok := req.Context().Value(refsLimitKey{}).(int64)
	if !ok || !strings.HasSuffix(req.URL.Path, "/info/refs") {
		return resp, nil
	}
	if resp.ContentLength > limit {
		resp.Body.Close()
		return nil, tooManyRefs(limit)
	}
	resp.Body = &limitedBody{body: resp.Body, limit: limit, remaining: limit}
	return resp, nil
}

// limitedBody reads a response body of at most limit bytes, and fails rather than reading past it.
type limitedBody struct {
	body io.ReadCloser
	// limit is the most the body may hold, and remaining what's left of it to read.
	limit, remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		// Read one byte more to tell a body that ends exactly at the limit from one that goes on.
		var probe [1]byte
		n, err := b.body.Read(probe[:])
		if n > 0 {
			return 0, tooManyRefs(b.limit)
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.body.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.body.Close() }

func tooManyRefs(limit int64) error {
	return fmt.Errorf("the repository advertises more than %d bytes of references, which ingestion won't read", limit)
}
