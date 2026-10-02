# Slice R2: brand

## Goal

Give Rulemart its own mark, from the brand package in `brand/`, everywhere the site shows one: the header, the
browser tab, a phone's home screen, links shared on social sites, and the README. One pull request, reviewed from
its before-and-after screenshots, so the look is judged on its own. [realignment.md](../realignment.md) explains
why it is a slice of its own.

## What a visitor sees

- **The header** reads Fabrica's cube and name, a slash, then Rulemart's symbol (three stacked planes) and name.
  The symbol is drawn in the current text color, so it follows the theme, and sits on the text's baseline at the
  wordmark's cap height, as Josh's local edit placed it.
- **The browser tab** shows the package's adaptive favicon, which changes with the browser's light or dark theme,
  with the ICO for browsers that take no SVG.
- **A phone's home screen** shows the package's apple-touch-icon.
- **A link shared** on a social site shows a 1200x630 image with the symbol and wordmark.
- **The README** opens with the horizontal logo, light and dark through a `<picture>` element.

## Decisions

- **Proposed: the package's files are copied unchanged into `internal/platform/web/static/`**: `favicon.svg`
  (adaptive), `favicon.ico`, `apple-touch-icon.png`, and the symbol as the header's inline SVG, drawn with
  `currentColor` so one file serves both themes. The package's `SHA256SUMS` stays in `brand/` as the record of
  what was copied.
- **Proposed: the social image is regenerated** from the package's dark horizontal logo on the site's paper color,
  at 1200x630, centered, with a script kept in `brand/` beside the package's `build.py`, so it can be regenerated
  when the brand changes.
- **Proposed: the header keeps Fabrica's cube** before Fabrica's name, as the prototype does, and adds Rulemart's
  symbol before Rulemart's name, 24 pixels tall at desktop width; on narrow screens, where Fabrica's name already
  hides, the symbol stays so the brand still reads as two marks.
- **Proposed: the Code Rules companion mark in `brand/code-rules/` is not used**, since the Code Rules site owns its
  own mark.
- **Existing:** static files are served under hashed names and cached for a year, so replacing them needs no
  infrastructure change; `/favicon.ico` has its own route.

## Not in this slice

- Any change to layout, type, color tokens, or copy beyond the marks.

## Verification

- `make check` and `make check-generated` pass; the icon tests cover the new SVG (no scripts, handlers, or
  external references).
- Before-and-after screenshots of the home page, a library page, and a rule page at 1280 and 390 pixels, light and
  dark, plus the browser tab and the social card preview, in the pull request.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
