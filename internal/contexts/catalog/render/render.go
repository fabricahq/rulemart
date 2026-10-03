// Package render renders a library's Markdown, a rule's body or a Markdown file among its assets, and its text files,
// as the HTML Rulemart's pages show, with links where domain.MarkdownSource says, within a byte allowance. Renderer implements
// domain.Renderer, which ingestion's assembly calls; only ingestion links this package, so the web function carries no
// Markdown parser or highlighter.
package render

import (
	"bytes"
	"fmt"
	"html"
	"path"
	"strings"
	"time"
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

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// Renderer renders a library's files as Markdown, Code, and Links do.
type Renderer struct{}

func (Renderer) Markdown(body string, source domain.MarkdownSource, allowance int64) (string, int64, error) {
	return Markdown(body, source, allowance)
}

func (Renderer) Code(text, file string, allowance int64) (string, int64, error) {
	return Code(text, file, allowance)
}

func (Renderer) Links(body string) []string { return Links(body) }

// pageKey carries the source being rendered to pageTransformer, and allowanceKey the allowance that pays for
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

// Markdown returns the HTML for body, the Markdown of the file from, and the bytes it used of allowance. It drops a
// leading heading that repeats the rule's title, points links and images where from's LinkURL and ImageURL say,
// highlights fenced code, and escapes raw HTML. goldmark's renderer already drops links with dangerous schemes, such
// as javascript:.
//
// A short body can expand, such as many references to one long link definition, so render counts what it builds
// as it goes, each rewritten link and each byte of HTML, and stops with domain.ErrOverAllowance rather than build past
// allowance. Highlighting some code takes chroma's lexers minutes, so a body gets highlightBudget for it, and code
// past the budget is shown escaped, without highlighting.
func Markdown(body string, from domain.MarkdownSource, allowance int64) (html string, used int64, err error) {
	spent := &spending{limit: allowance}
	context := parser.NewContext()
	context.Set(pageKey, from)
	context.Set(allowanceKey, spent)
	source := []byte(body)
	document := markdown.Parser().Parse(text.NewReader(source), parser.WithContext(context))
	if err, refused := context.Get(refusedKey).(error); refused {
		return "", 0, err
	}
	out := allowanceWriter{spent: spent, highlightUntil: time.Now().Add(highlightBudget)}
	err = htmlRenderer.Render(&out, source, document)
	if out.err != nil {
		return "", 0, out.err
	}
	if err != nil {
		return "", 0, fmt.Errorf("render Markdown: %v", err)
	}
	return out.html.String(), spent.used, nil
}

// spending counts the bytes one render builds, up to its allowance.
type spending struct {
	limit, used int64
}

// spend records n more bytes, or refuses them when they would pass the limit.
func (s *spending) spend(n int64) error {
	if s.used+n > s.limit {
		return domain.ErrOverAllowance
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
	// highlightUntil is when the render stops highlighting code, which it then shows escaped.
	highlightUntil time.Time
}

// highlightBudget is how long one rule's code may take to highlight. Real rules take milliseconds, but chroma's
// lexers take time quadratic in some inputs, such as a long run of one short Java token.
const highlightBudget = time.Second

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

// Code returns the HTML for the text of file as a block of code, highlighted when chroma knows file's language by its
// name, within highlightBudget, and the bytes it used of allowance, failing with domain.ErrOverAllowance, without
// building past allowance, when the HTML would need more.
func Code(text, file string, allowance int64) (string, int64, error) {
	out := allowanceWriter{spent: &spending{limit: allowance}, highlightUntil: time.Now().Add(highlightBudget)}
	language := strings.TrimPrefix(strings.ToLower(path.Ext(file)), ".")
	_, _ = out.WriteString("<pre><code")
	if language != "" {
		_, _ = out.WriteString(` class="language-` + html.EscapeString(language) + `"`)
	}
	_, _ = out.WriteString(">")
	var lexer chroma.Lexer
	if text != "" {
		lexer = lexers.Match(path.Base(file))
	}
	writeHighlightedWith(&out, lexer, text, out.highlightUntil)
	_, _ = out.WriteString("</code></pre>\n")
	if out.err != nil {
		return "", 0, out.err
	}
	return out.html.String(), out.spent.used, nil
}

// Links returns the destinations of body's links and images, as written, in the order they appear: the links a page
// of it would show, including those that name a link definition, without rewriting them.
func Links(body string) []string {
	source := []byte(body)
	document := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(source))
	var destinations []string
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Link:
			destinations = append(destinations, string(node.Destination))
		case *ast.Image:
			destinations = append(destinations, string(node.Destination))
		}
		return ast.WalkContinue, nil
	})
	return destinations
}

// pageTransformer adapts a parsed body to its page, using the source in the parser context.
type pageTransformer struct{}

func (pageTransformer) Transform(document *ast.Document, reader text.Reader, context parser.Context) {
	page := context.Get(pageKey).(domain.MarkdownSource)
	spent := context.Get(allowanceKey).(*spending)
	source := reader.Source()
	if heading, ok := document.FirstChild().(*ast.Heading); ok && page.Title != "" && strings.TrimSpace(string(heading.Text(source))) == strings.TrimSpace(page.Title) {
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
			node.Destination, err = rewrite(links, node.Destination, page.LinkURL)
		case *ast.Image:
			node.Destination, err = rewrite(images, node.Destination, page.ImageURL)
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
	writeHighlighted(w, language, code.String(), w.(*allowanceWriter).highlightUntil)
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// writeHighlighted writes code as escaped HTML, wrapping tokens in highlight classes when chroma has a lexer for
// language, until the time is past until. It writes whatever code remains then escaped, without highlighting.
func writeHighlighted(w util.BufWriter, language, code string, until time.Time) {
	var lexer chroma.Lexer
	if language != "" {
		lexer = lexers.Get(language)
	}
	writeHighlightedWith(w, lexer, code, until)
}

// writeHighlightedWith writes code as escaped HTML, wrapping tokens in highlight classes when lexer isn't nil, until
// the time is past until. It writes whatever code remains then escaped, without highlighting.
func writeHighlightedWith(w util.BufWriter, lexer chroma.Lexer, code string, until time.Time) {
	if lexer == nil || time.Now().After(until) {
		_, _ = w.WriteString(html.EscapeString(code))
		return
	}
	// chroma turns line endings into \n before it lexes, so the tokens it returns spell code with them turned too.
	code = lineEndings.Replace(code)
	tokens, err := lexer.Tokenise(nil, code)
	if err != nil {
		_, _ = w.WriteString(html.EscapeString(code))
		return
	}
	// Tokens are taken one at a time, so the budget is checked between them, and runs of tokens with one class are
	// written as one span.
	written, pending, pendingClass := 0, strings.Builder{}, ""
	flush := func() {
		if pending.Len() == 0 {
			return
		}
		if pendingClass == "" {
			_, _ = w.WriteString(html.EscapeString(pending.String()))
		} else {
			_, _ = w.WriteString(`<span class="` + pendingClass + `">` + html.EscapeString(pending.String()) + "</span>")
		}
		pending.Reset()
	}
	for token := tokens(); token != chroma.EOF; token = tokens() {
		if class := highlightClass(token.Type); class != pendingClass {
			flush()
			pendingClass = class
		}
		pending.WriteString(token.Value)
		written += len(token.Value)
		if time.Now().After(until) {
			break
		}
	}
	flush()
	if written < len(code) {
		_, _ = w.WriteString(html.EscapeString(code[written:]))
	}
}

// lineEndings turns \r\n and \r into \n, as chroma does before it lexes.
var lineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

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
