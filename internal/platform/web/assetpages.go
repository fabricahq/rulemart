// A rule's assets: the page of each one, the list of them beside a rule, and the images Rulemart serves.

package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// assetCache lets browsers and CloudFront keep an image for a day: a library release that changes it shows within one,
// and its page links it again.
const assetCache = "public, max-age=86400"

// assetPolicy is the content security policy of an image Rulemart serves, in place of a page's: it loads nothing,
// runs nothing, and opens in a sandbox, so an SVG a library wrote can't act as a page of Rulemart's, even opened on its
// own.
const assetPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// ruleParam names the rule whose page a shared asset's page shows it with.
const ruleParam = "rule"

// requestedAsset returns the path in the repository of the asset r's path names, and whether it names one: a shared
// asset, under the library's shared asset directory, or one of a rule's own, under the rule's ID, which then names
// rulePath. Code Rules reserves assets, so no group or rule's ID holds it.
func requestedAsset(r *http.Request) (rulePath, assetPath string, ok bool) {
	if r.PathValue("kind")+"/" == domain.SharedAssetDir {
		return "", domain.SharedAssetDir + r.PathValue("group"), true
	}
	rule := r.PathValue("rule")
	if strings.HasPrefix(rule, domain.SharedAssetDir) {
		return "", rule, true
	}
	if rulePath, file, own := strings.Cut(rule, "/assets/"); own {
		return rulePath, domain.RuleAssetDir(rulePath) + file, true
	}
	return "", "", false
}

// orAsset returns a handler of the library group and rule pages' paths that shows the asset r's path names, when it
// names one, and otherwise calls next. A shared asset directly under the library's shared asset directory has a path
// of a library group's shape, whose kind is assets, and any other asset a path of a rule's.
func (s *server) orAsset(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rulePath, assetPath, ok := requestedAsset(r); ok {
			s.asset(w, r, rulePath, assetPath)
			return
		}
		next(w, r)
	}
}

// asset shows the asset at assetPath, a path in the repository, of the rule at rulePath on a page of its own, or with
// the raw parameter, serves it, when it's an image Rulemart keeps. A shared asset's page shows it with the rule the rule
// parameter names, or the first rule that lists it.
func (s *server) asset(w http.ResponseWriter, r *http.Request, rulePath, assetPath string) {
	owner, repo := r.PathValue("owner"), r.PathValue("repo")
	query := r.URL.Query()
	if query.Has(domain.AssetRawParam) {
		s.assetImage(w, r, owner, repo, assetPath)
		return
	}
	shared := rulePath == ""
	if shared {
		rulePath = query.Get(ruleParam)
	}
	page, err := s.catalog.AssetPage(r.Context(), owner, repo, rulePath, assetPath)
	if shared && rulePath != "" && errors.Is(err, app.ErrNotFound) {
		// The rule doesn't list it, or there's no such rule: show it with the first rule that does.
		page, err = s.catalog.AssetPage(r.Context(), owner, repo, "", assetPath)
	}
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	lib := page.Page.Library
	if lib.Owner != owner || lib.Name != repo {
		target := url.URL{Path: assetPagePath(newLibraryView(lib), page.Page.Rule.Path, assetPath), RawQuery: r.URL.RawQuery}
		redirect(w, r, target.String())
		return
	}
	rule := newRuleView(newLibraryView(lib), page.Page)
	rule.groupIcon = newGroupIcon(page.Page.Rule.CanonicalGroup, s.assets.iconURL)
	assets := newAssetViews(rule, page.Page.Assets, assetPath)
	current := assets[slices.IndexFunc(assets, func(a assetView) bool { return a.current })]
	html := ruleContext(page.HTML, rule.library, rule.id)
	if !rule.library.vetted {
		html = untrustedLinks(html)
	}
	canonical := assetPagePath(rule.library, rule.id, assetPath)
	s.render(w, r, http.StatusOK, assetPage(s.pageChrome(canonical), rule, current, assets, html))
}

// assetImage serves the image at assetPath in the library owner/repo that Rulemart keeps, as the type ingestion
// recorded, for a day, under a policy that runs and loads nothing, or the missing page.
func (s *server) assetImage(w http.ResponseWriter, r *http.Request, owner, repo, assetPath string) {
	image, err := s.catalog.AssetImage(r.Context(), owner, repo, assetPath)
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	header := w.Header()
	header.Set("Content-Type", image.MediaType)
	header.Set("Content-Length", strconv.Itoa(len(image.Content)))
	header.Set("Content-Security-Policy", assetPolicy)
	header.Set("Cache-Control", assetCache)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(image.Content)
	}
}

// assetPagePath returns the path of the page of the asset at assetPath, of the rule at rulePath in lib, without the
// rule a shared asset's page names.
func assetPagePath(lib libraryView, rulePath, assetPath string) string {
	return domain.AssetPagePath(lib.fullName(), rulePath, assetPath)
}

// assetView is an asset as the list of a rule's assets, and its page, show it.
type assetView struct {
	// path is the file's path in the repository, and name its path in the rule's asset directory or the library's
	// shared one, as the list names it.
	path, name string
	// href is the asset's page, which names the rule for a shared asset.
	href string
	size string
	kind domain.AssetKind
	// shared marks a file in the library's shared asset directory, which no rule's version covers.
	shared bool
	// current marks the asset whose page shows the list.
	current bool
	// kept reports whether Rulemart keeps the file's bytes, and image is where it serves an image it keeps.
	kept  bool
	image string
	// release is the library release whose commit the copy is from, and githubURL and rawURL the file there on GitHub.
	release           string
	githubURL, rawURL string
}

// newAssetViews describes assets, the assets of the rule r, marking the one at current, if any.
func newAssetViews(r ruleView, assets []views.Asset, current string) []assetView {
	list := make([]assetView, len(assets))
	for i, a := range assets {
		tag := domain.ReleaseTag(a.Release)
		v := assetView{
			path: a.Path, href: assetPagePath(r.library, r.id, a.Path), size: formatSize(a.Size),
			kind: domain.AssetKindOf(a.MediaType), current: a.Path == current, kept: a.Kept, release: tag,
			githubURL: domain.BlobURL(r.library.fullName(), tag, a.Path), rawURL: domain.RawURL(r.library.fullName(), tag, a.Path),
		}
		v.name, v.shared = strings.CutPrefix(a.Path, domain.SharedAssetDir)
		if !v.shared {
			v.name = strings.TrimPrefix(a.Path, domain.RuleAssetDir(r.id))
		} else {
			v.href += "?" + ruleParam + "=" + ruleQuery(r.id)
		}
		if v.kind == domain.AssetImage && a.Kept {
			v.image = domain.AssetImagePath(assetPagePath(r.library, r.id, a.Path))
		}
		list[i] = v
	}
	return list
}

// fileName returns the asset's name without its directories.
func (a assetView) fileName() string { return path.Base(a.path) }

// ruleQuery returns a rule's ID as a query's value, keeping its slashes, which a query may hold.
func ruleQuery(rulePath string) string {
	return strings.ReplaceAll(url.QueryEscape(rulePath), "%2F", "/")
}

// ruleContext returns rendered, HTML ingestion's renderer wrote for the rule at rulePath in lib, with each link to a
// shared asset's page naming the rule, so that page shows the asset as the rule's. The renderer escapes every < in
// text and writes each link's href itself, so only its links match.
func ruleContext(rendered string, lib libraryView, rulePath string) string {
	shared := regexp.MustCompile(`href="(` + regexp.QuoteMeta(lib.href+"/"+domain.SharedAssetDir) + `[^"#?]*)(#[^"]*)?"`)
	return shared.ReplaceAllString(rendered, `href="$1?`+ruleParam+`=`+ruleQuery(rulePath)+`$2"`)
}

// formatSize writes a file's size as pages show it, such as 312 B, 2.4 KB, or 1.2 MB, counting 1,024 bytes a KB.
func formatSize(bytes int64) string {
	switch {
	case bytes < 1<<10:
		return strconv.FormatInt(bytes, 10) + " B"
	case bytes < 1<<20:
		return scaled(bytes, 1<<10) + " KB"
	}
	return scaled(bytes, 1<<20) + " MB"
}

// scaled returns bytes in units of unit, to one decimal place below 100, and whole above.
func scaled(bytes, unit int64) string {
	if value := float64(bytes) / float64(unit); value < 100 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0")
	}
	return strconv.FormatInt((bytes+unit/2)/unit, 10)
}
