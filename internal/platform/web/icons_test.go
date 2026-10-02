package web

import (
	"bytes"
	"encoding/xml"
	"errors"
	"image/png"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// Pages show group icons and the favicon with <img> and <link>, which run nothing, but a visitor can open a static
// SVG's URL, where it is a document on Rulemart's origin. So a static SVG, vendored or Rulemart's own, may hold only
// drawing: no scripts, event handlers, embedded documents, or references outside itself.
func TestStaticSVGsHoldOnlyDrawing(t *testing.T) {
	count := 0
	err := fs.WalkDir(embedded, "static", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || path.Ext(name) != ".svg" {
			return err
		}
		count++
		content, err := fs.ReadFile(embedded, name)
		if err != nil {
			return err
		}
		if problem := activeSVGContent(content); problem != "" {
			t.Errorf("%s: %s", name, problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("found no static SVGs")
	}
}

// A PNG icon, for a project whose only published logo is one, holds no code, but it must be a PNG, downscaled to at
// most 256 pixels a side, since pages draw icons far smaller and every browse page loads them.
func TestVendoredPNGIconsAreSmallPNGs(t *testing.T) {
	err := fs.WalkDir(embedded, "static/icons", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || path.Ext(name) != ".png" {
			return err
		}
		content, err := fs.ReadFile(embedded, name)
		if err != nil {
			return err
		}
		config, err := png.DecodeConfig(bytes.NewReader(content))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return nil
		}
		if _, err := png.Decode(bytes.NewReader(content)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if config.Width > 256 || config.Height > 256 {
			t.Errorf("%s is %dx%d, want at most 256 pixels a side", name, config.Width, config.Height)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActiveSVGContentFindsWhatCouldRunOrLoad(t *testing.T) {
	for name, drawing := range map[string]string{
		"a plain drawing": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><path id="a" d="M0 0h24"/></defs>` +
			`<use href="#a" fill="url(#a)" style="stroke:url( '#a')"/></svg>`,
		"a stylesheet that adapts to the theme": `<svg><style>path{stroke:#1c1c1c}@media(prefers-color-scheme:dark)` +
			`{path{stroke:#f2f2f2;fill:url(#a)}}</style><path d="M0 0"/></svg>`,
	} {
		if problem := activeSVGContent([]byte(drawing)); problem != "" {
			t.Errorf("%s was rejected: %s", name, problem)
		}
	}
	for name, svg := range map[string]string{
		"a script":                `<svg><script>alert(1)</script></svg>`,
		"an event handler":        `<svg onload="alert(1)"><path d="M0 0"/></svg>`,
		"an embedded document":    `<svg><foreignObject><div>hi</div></foreignObject></svg>`,
		"a link to a page":        `<svg><a href="https://example.com"><path d="M0 0"/></a></svg>`,
		"another file":            `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><image xlink:href="https://example.com/x.png"/></svg>`,
		"a script URL":            `<svg><use href="javascript:alert(1)"/></svg>`,
		"a style loading a URL":   `<svg><path style="fill:url(https://example.com/x)" d="M0 0"/></svg>`,
		"a URL after a fragment":  `<svg><path style="fill:url(#a);stroke:url('https://example.com/x')" d="M0 0"/></svg>`,
		"a stylesheet import":     `<svg><style>@import url(https://example.com/x.css);</style></svg>`,
		"a bare import":           `<svg><style>@IMPORT "x.css";</style></svg>`,
		"a stylesheet URL":        `<svg><style>path{fill:url(https://example.com/x)}</style></svg>`,
		"an escaped stylesheet":   `<svg><style>path{fill:u\72l(https://example.com/x)}</style></svg>`,
		"an element in a style":   `<svg><style><script>alert(1)</script></style></svg>`,
		"a style's event handler": `<svg><style onload="alert(1)">path{stroke:red}</style></svg>`,
		"an entity declaration":   `<!DOCTYPE svg [<!ENTITY x "y">]><svg></svg>`,
		"malformed XML":           `<svg><path></svg>`,
	} {
		if activeSVGContent([]byte(svg)) == "" {
			t.Errorf("%s was accepted", name)
		}
	}
}

// externalURL matches a CSS url() that refers to anything but a fragment of the same document, such as a gradient.
var externalURL = regexp.MustCompile(`(?i)url\(\s*['"]?[^#'"\s]`)

// activeSVGContent returns what in an SVG could run code or load anything, or "" when it holds only drawing.
func activeSVGContent(svg []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(svg))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return ""
		}
		if err != nil {
			return "malformed XML: " + err.Error()
		}
		switch t := token.(type) {
		case xml.Directive:
			return "a directive, such as a DOCTYPE"
		case xml.StartElement:
			switch t.Name.Local {
			case "script", "foreignObject", "a", "iframe", "embed", "object", "image", "feImage":
				return "a <" + t.Name.Local + "> element"
			}
			if problem := activeAttribute(t.Attr); problem != "" {
				return problem
			}
			if t.Name.Local == "style" {
				if problem := activeStyle(decoder); problem != "" {
					return problem
				}
			}
		}
	}
}

// activeAttribute returns what among an element's attributes could run code or load anything, or "" when none can.
func activeAttribute(attrs []xml.Attr) string {
	for _, attr := range attrs {
		name, value := strings.ToLower(attr.Name.Local), strings.TrimSpace(attr.Value)
		switch {
		case strings.HasPrefix(name, "on"):
			return "an event handler, " + attr.Name.Local
		case name == "href" && !strings.HasPrefix(value, "#"):
			return "a reference to " + value
		case externalURL.MatchString(value):
			return "a URL in " + attr.Name.Local
		}
	}
	return ""
}

// activeStyle reads a <style> element's content from decoder, just past its start, and returns what in it could load
// anything, or "" when it only styles the drawing, as a theme's colors do. It takes no escapes, which could spell an
// import or a URL the checks don't see.
func activeStyle(decoder *xml.Decoder) string {
	var css strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "malformed XML: " + err.Error()
		}
		switch t := token.(type) {
		case xml.CharData:
			css.Write(t)
		case xml.StartElement:
			return "a <" + t.Name.Local + "> element in a <style>"
		case xml.EndElement:
			text := css.String()
			switch {
			case strings.Contains(text, `\`):
				return "an escape in a <style>"
			case strings.Contains(strings.ToLower(text), "@import"):
				return "an @import in a <style>"
			case externalURL.MatchString(text):
				return "a URL in a <style>"
			}
			return ""
		}
	}
}
