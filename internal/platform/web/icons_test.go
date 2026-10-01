package web

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// Pages show group icons with <img>, which runs nothing, but a visitor can open an icon's URL, where an SVG is a
// document on Rulemart's origin. So a vendored icon may hold only drawing: no scripts, event handlers, embedded
// documents, or references outside itself.
func TestVendoredIconsHoldOnlyDrawing(t *testing.T) {
	count := 0
	err := fs.WalkDir(embedded, "static/icons", func(name string, entry fs.DirEntry, err error) error {
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
		t.Fatal("found no vendored icons")
	}
}

func TestActiveSVGContentFindsWhatCouldRunOrLoad(t *testing.T) {
	const drawing = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><path id="a" d="M0 0h24"/></defs>` +
		`<use href="#a" fill="url(#a)" style="stroke:url( '#a')"/></svg>`
	if problem := activeSVGContent([]byte(drawing)); problem != "" {
		t.Fatalf("a plain drawing was rejected: %s", problem)
	}
	for name, svg := range map[string]string{
		"a script":               `<svg><script>alert(1)</script></svg>`,
		"an event handler":       `<svg onload="alert(1)"><path d="M0 0"/></svg>`,
		"an embedded document":   `<svg><foreignObject><div>hi</div></foreignObject></svg>`,
		"a link to a page":       `<svg><a href="https://example.com"><path d="M0 0"/></a></svg>`,
		"another file":           `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><image xlink:href="https://example.com/x.png"/></svg>`,
		"a script URL":           `<svg><use href="javascript:alert(1)"/></svg>`,
		"a style loading a URL":  `<svg><path style="fill:url(https://example.com/x)" d="M0 0"/></svg>`,
		"a URL after a fragment": `<svg><path style="fill:url(#a);stroke:url('https://example.com/x')" d="M0 0"/></svg>`,
		"a stylesheet":           `<svg><style>@import url(https://example.com/x.css);</style></svg>`,
		"an entity declaration":  `<!DOCTYPE svg [<!ENTITY x "y">]><svg></svg>`,
		"malformed XML":          `<svg><path></svg>`,
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
			case "script", "foreignObject", "style", "a", "iframe", "embed", "object", "image", "feImage":
				return "a <" + t.Name.Local + "> element"
			}
			for _, attr := range t.Attr {
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
		}
	}
}
