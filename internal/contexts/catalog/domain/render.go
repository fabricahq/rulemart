// Render a rule's Markdown body as the HTML its Rulemart page shows.

package domain

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// rulePage is where a rule's Markdown came from, which its relative links and images resolve against.
type rulePage struct {
	// repository is the library's GitHub repository, as owner/name.
	repository string
	// path is the rule's file, such as practices/testing/verify-retry-limits.md.
	path string
	// title is the rule's title, which the page shows above the body.
	title string
	// tag published the rule's version, and latestTag is the library's latest release. Files the rule's version
	// covers, its Markdown and its own assets, link to tag; library-wide files link to latestTag.
	tag, latestTag string
}

// pageKey carries the rulePage being rendered to pageTransformer, and allowanceKey the allowance that pays for
// rewritten links. refusedKey holds the allowance's error when rewriting links would pass it.
var (
	pageKey      = parser.NewContextKey()
	allowanceKey = parser.NewContextKey()
	refusedKey   = parser.NewContextKey()
)

// markdown parses rule bodies: CommonMark with GitHub's extensions, adapted to the rule's page.
var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
		parser.WithASTTransformers(util.Prioritized(pageTransformer{}, 100)),
	),
)

// htmlRenderer renders what markdown parses: goldmark's HTML for CommonMark and GitHub's extensions, and
// ruleNodeRenderer's raw HTML as text and highlighted code. Every node it renders first checks the allowanceWriter
// it writes to, and stops the whole render once the allowance is spent, so nothing past it is escaped or built.
// It names GitHub's extensions' renderers itself, so it can wrap them; a parser extension added to markdown needs
// its renderer added here.
var htmlRenderer = renderer.NewRenderer(renderer.WithNodeRenderers(
	util.Prioritized(stopAtAllowance{goldmarkhtml.NewRenderer()}, 1000),
	util.Prioritized(stopAtAllowance{extension.NewTableHTMLRenderer()}, 500),
	util.Prioritized(stopAtAllowance{extension.NewStrikethroughHTMLRenderer()}, 500),
	util.Prioritized(stopAtAllowance{extension.NewTaskCheckBoxHTMLRenderer()}, 500),
	util.Prioritized(stopAtAllowance{ruleNodeRenderer{}}, 100),
))

// stopAtAllowance is a node renderer whose functions stop the render once its allowanceWriter has refused a write.
type stopAtAllowance struct {
	renderer.NodeRenderer
}

func (s stopAtAllowance) RegisterFuncs(registerer renderer.NodeRendererFuncRegisterer) {
	s.NodeRenderer.RegisterFuncs(allowanceRegisterer{registerer})
}

// allowanceRegisterer registers each node rendering function behind a check of the allowance.
type allowanceRegisterer struct {
	renderer.NodeRendererFuncRegisterer
}

func (r allowanceRegisterer) Register(kind ast.NodeKind, renderNode renderer.NodeRendererFunc) {
	r.NodeRendererFuncRegisterer.Register(kind, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		out := w.(*allowanceWriter)
		if out.err != nil {
			return ast.WalkStop, out.err
		}
		status, err := renderNode(w, source, node, entering)
		if out.err != nil {
			return ast.WalkStop, out.err
		}
		return status, err
	})
}

// render returns the HTML for a rule's Markdown body, and the bytes it used of allowance. It drops a leading
// heading that repeats the title, points relative links and images at the files on GitHub at the release that
// holds them, highlights fenced code, and escapes raw HTML. goldmark's renderer already drops links with dangerous
// schemes, such as javascript:.
//
// A short body can expand, such as many references to one long link definition, so render counts what it builds
// as it goes, each rewritten link and each byte of HTML, and stops with errOverAllowance rather than build past
// allowance.
func render(body string, page rulePage, allowance int64) (html string, used int64, err error) {
	spent := &spending{limit: allowance}
	context := parser.NewContext()
	context.Set(pageKey, page)
	context.Set(allowanceKey, spent)
	source := []byte(body)
	document := markdown.Parser().Parse(text.NewReader(source), parser.WithContext(context))
	if err, refused := context.Get(refusedKey).(error); refused {
		return "", 0, err
	}
	out := allowanceWriter{spent: spent}
	err = htmlRenderer.Render(&out, source, document)
	if out.err != nil {
		return "", 0, out.err
	}
	if err != nil {
		return "", 0, fmt.Errorf("render Markdown: %v", err)
	}
	return out.html.String(), spent.used, nil
}

// errOverAllowance reports a render that would build more than its allowance.
var errOverAllowance = errors.New("the rule's HTML needs more bytes than its allowance")

// spending counts the bytes one render builds, up to its allowance.
type spending struct {
	limit, used int64
}

// spend records n more bytes, or refuses them when they would pass the limit.
func (s *spending) spend(n int64) error {
	if s.used+n > s.limit {
		return errOverAllowance
	}
	s.used += n
	return nil
}

// allowanceWriter collects rendered HTML, spending the render's allowance on every write, and refuses every write
// once one would pass it. It writes straight through, as a util.BufWriter, so the renderer adds no buffer of its
// own between them.
type allowanceWriter struct {
	spent *spending
	html  bytes.Buffer
	// err is the allowance's refusal, once a write was refused.
	err error
}

func (w *allowanceWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if err := w.spent.spend(int64(len(p))); err != nil {
		w.err = err
		return 0, err
	}
	return w.html.Write(p)
}

func (w *allowanceWriter) WriteString(text string) (int, error) {
	return w.Write([]byte(text))
}

func (w *allowanceWriter) WriteByte(c byte) error {
	_, err := w.Write([]byte{c})
	return err
}

func (w *allowanceWriter) WriteRune(r rune) (int, error) {
	return w.Write(utf8.AppendRune(nil, r))
}

// Flush, Available, and Buffered complete util.BufWriter: nothing is buffered.
func (w *allowanceWriter) Flush() error   { return w.err }
func (w *allowanceWriter) Available() int { return 0 }
func (w *allowanceWriter) Buffered() int  { return 0 }

// pageTransformer adapts a parsed rule body to its page, using the rulePage in the parser context.
type pageTransformer struct{}

func (pageTransformer) Transform(document *ast.Document, reader text.Reader, context parser.Context) {
	page := context.Get(pageKey).(rulePage)
	spent := context.Get(allowanceKey).(*spending)
	source := reader.Source()
	if heading, ok := document.FirstChild().(*ast.Heading); ok && strings.TrimSpace(string(heading.Text(source))) == strings.TrimSpace(page.title) {
		document.RemoveChild(document, heading)
	}
	// References to one definition share its destination, so each distinct destination is rewritten, and paid
	// for, once.
	links, images := map[string][]byte{}, map[string][]byte{}
	rewrite := func(rewritten map[string][]byte, destination []byte, to func(string) string) ([]byte, error) {
		if url, ok := rewritten[string(destination)]; ok {
			return url, nil
		}
		url := to(string(destination))
		if err := spent.spend(int64(len(url))); err != nil {
			return nil, err
		}
		rewritten[string(destination)] = []byte(url)
		return rewritten[string(destination)], nil
	}
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var err error
		switch node := node.(type) {
		case *ast.Link:
			node.Destination, err = rewrite(links, node.Destination, page.linkURL)
		case *ast.Image:
			node.Destination, err = rewrite(images, node.Destination, page.imageURL)
		}
		if err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		context.Set(refusedKey, err)
	}
}

// linkURL returns where a link in the rule leads: a relative destination opens the file on GitHub, and anything
// else, such as an absolute URL or a fragment, stays as written.
func (p rulePage) linkURL(destination string) string {
	file, suffix, ok := p.resolve(destination)
	if !ok {
		return destination
	}
	if file == "" {
		return TreeURL(p.repository, p.tagFor(file)) + suffix
	}
	return BlobURL(p.repository, p.tagFor(file), file) + suffix
}

// imageURL returns where an image in the rule loads from: a relative source loads the file from GitHub, and an
// absolute one stays as written.
func (p rulePage) imageURL(source string) string {
	file, suffix, ok := p.resolve(source)
	if !ok || file == "" {
		return source
	}
	return RawURL(p.repository, p.tagFor(file), file) + suffix
}

// resolve returns the repository file a relative destination names, resolved against the rule's directory, and
// the query and fragment to keep after it. file is empty for the repository root, including for destinations that
// climb above it. ok is false for a destination that isn't a relative path: one with a scheme or host, or only a
// query or fragment.
func (p rulePage) resolve(destination string) (file, suffix string, ok bool) {
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" {
		return "", "", false
	}
	if u.RawQuery != "" {
		suffix += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		suffix += "#" + u.EscapedFragment()
	}
	file = path.Clean(u.Path)
	if !strings.HasPrefix(u.Path, "/") {
		file = path.Join(path.Dir(p.path), u.Path)
	}
	file = strings.TrimPrefix(file, "/")
	if file == "." || file == ".." || strings.HasPrefix(file, "../") {
		file = ""
	}
	return file, suffix, true
}

// tagFor returns the release whose tree holds file as the rule shows it: the rule's own release for its Markdown
// and its asset directory, assets/<rule name>/ beside it, and the latest release for everything else.
func (p rulePage) tagFor(file string) string {
	dir, name := path.Split(strings.TrimSuffix(p.path, ".md"))
	if file == p.path || strings.HasPrefix(file, dir+"assets/"+name+"/") {
		return p.tag
	}
	return p.latestTag
}

// ruleNodeRenderer renders the nodes Rulemart shows differently from goldmark: raw HTML as escaped text, and fenced
// code with syntax highlighting.
type ruleNodeRenderer struct{}

func (ruleNodeRenderer) RegisterFuncs(registerer renderer.NodeRendererFuncRegisterer) {
	registerer.Register(ast.KindHTMLBlock, renderHTMLBlock)
	registerer.Register(ast.KindRawHTML, renderRawHTML)
	registerer.Register(ast.KindFencedCodeBlock, renderFencedCode)
}

// renderHTMLBlock shows a block of raw HTML as preformatted text.
func renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	block := node.(*ast.HTMLBlock)
	_, _ = w.WriteString("<pre><code>")
	lines := block.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		_, _ = w.WriteString(html.EscapeString(string(line.Value(source))))
	}
	if block.HasClosure() {
		_, _ = w.WriteString(html.EscapeString(string(block.ClosureLine.Value(source))))
	}
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// renderRawHTML shows inline raw HTML, such as <br>, as text.
func renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	segments := node.(*ast.RawHTML).Segments
	for i := range segments.Len() {
		segment := segments.At(i)
		_, _ = w.WriteString(html.EscapeString(string(segment.Value(source))))
	}
	return ast.WalkSkipChildren, nil
}

// renderFencedCode writes a fenced code block, with the tokens of a language chroma knows wrapped in highlight
// classes. The page's styles color the classes for light and dark themes.
func renderFencedCode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	block := node.(*ast.FencedCodeBlock)
	var code strings.Builder
	lines := block.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		code.Write(line.Value(source))
	}
	language := string(block.Language(source))
	_, _ = w.WriteString("<pre><code")
	if language != "" {
		_, _ = w.WriteString(` class="language-` + html.EscapeString(language) + `"`)
	}
	_, _ = w.WriteString(">")
	writeHighlighted(w, language, code.String())
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// writeHighlighted writes code as escaped HTML, wrapping tokens in highlight classes when chroma has a lexer for
// language.
func writeHighlighted(w util.BufWriter, language, code string) {
	lexer := lexers.Get(language)
	if language == "" || lexer == nil {
		_, _ = w.WriteString(html.EscapeString(code))
		return
	}
	tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		_, _ = w.WriteString(html.EscapeString(code))
		return
	}
	for token := tokens(); token != chroma.EOF; token = tokens() {
		class := highlightClass(token.Type)
		if class == "" {
			_, _ = w.WriteString(html.EscapeString(token.Value))
			continue
		}
		_, _ = w.WriteString(`<span class="` + class + `">` + html.EscapeString(token.Value) + "</span>")
	}
}

// highlightClass maps a chroma token to one of the five colors Code Rules' documentation uses for code, or to none.
func highlightClass(token chroma.TokenType) string {
	switch {
	case token == chroma.KeywordConstant, token == chroma.Literal, token.InSubCategory(chroma.LiteralNumber),
		token == chroma.NameConstant, token == chroma.NameAttribute:
		return "hl-constant"
	case token == chroma.KeywordType, token.InSubCategory(chroma.NameFunction), token.InSubCategory(chroma.NameBuiltin),
		token == chroma.NameClass, token == chroma.NameTag, token == chroma.GenericHeading, token == chroma.GenericSubheading:
		return "hl-function"
	case token.InCategory(chroma.Keyword):
		return "hl-keyword"
	case token.InSubCategory(chroma.LiteralString):
		return "hl-string"
	case token.InCategory(chroma.Comment):
		return "hl-comment"
	}
	return ""
}
