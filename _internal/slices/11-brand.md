# Slice R2: brand

## Goal

Give Rulemart its own mark, from the brand package in `brand/`, everywhere the site shows one: the header, the
browser tab, a phone's home screen, links shared on social sites, and the README. One pull request, reviewed from
its before-and-after screenshots, so the look is judged on its own. [realignment.md](../realignment.md) explains
why it is a slice of its own.

Decisions marked **Proposed** are new in this slice and wait for review. **Existing** ones describe what the site
already does.

## What a visitor sees

- **The header** reads Fabrica's cube and name, a slash, then Rulemart's symbol (three stacked planes) and name.
  The symbol is drawn in the current text color, so it follows the theme, and is centered on the name's line, as
  Josh's local edit placed it: its drawing is about 16 pixels tall, a little taller than the name's capitals, so it
  reaches just above them and just below the baseline. It is 24 pixels at 720 pixels wide and up, 20 on a phone.
- **The browser tab** shows the package's adaptive favicon, whose stroke is dark in the browser's light theme and
  light in its dark theme, with the ICO, the symbol on a dark tile, for browsers that take no SVG.
- **A phone's home screen** shows the package's apple-touch-icon, the symbol on a dark tile.
- **A link shared** on a social site shows a 1200x630 image of the dark horizontal logo, symbol and wordmark,
  centered on the surface color.
- **The README** opens with the horizontal logo, dark or white with the reader's theme, through a `<picture>`
  element.

## Decisions

- **Proposed: the symbol goes before Rulemart's name in the header, 24 pixels tall, with the heavier stroke**
  (3.5 in the symbol's 64-unit box, against the logos' 3), as his local edit drew it; the brand guide gives the
  site's small sizes the heavier stroke. The header draws the package's own path, from `brand/source/rulemart.svg`,
  which is the path his edit used.
- **Proposed: the header keeps Fabrica's cube** before Fabrica's name, as the prototype does.
- **Proposed: on a phone, below 720 pixels, the symbol is 20 pixels and the header's parts sit closer**: 6 pixels
  rather than 8 between Fabrica's name, the slash, and Rulemart's, and 8 rather than 12 between the name and the
  buttons. At 24 pixels the Sign in button, or a signed-in visitor's account menu, crossed the right gutter by up to
  12 pixels between 384 and 400 pixels wide, where Fabrica's cube and name still show; now the header ends on the
  gutter at every width from 320 up. Below 384 pixels, where Fabrica's name already hides (**Existing**), the symbol
  stays.
- **Proposed: the favicons and touch icon are the package's files, copied unchanged** into
  `internal/platform/web/static/` under the names the pages and the `/favicon.ico` route already use (**Existing**):
  `favicon.svg` (adaptive), `favicon.ico`, and `apple-touch-icon.png`. Static files are served under hashed names
  and cached for a year, so replacing them needs no infrastructure change (**Existing**); `/favicon.ico` is cached a
  day.
- **Proposed: a static SVG may hold a `<style>` that loads nothing.** The adaptive favicon switches its stroke with a
  `prefers-color-scheme` rule, which the check for static SVGs rejected outright. A stylesheet runs no code; it can
  only load through an `@import` or a URL. So the check takes a `<style>` with neither, nor an escape that could spell
  one, nor an element inside it, and it now covers every static SVG, the favicon included, rather than only the
  vendored group icons.
- **Proposed: `brand/` keeps the package's SVGs, its guide, `build.py`, and the PNGs up to 400 pixels wide.** The
  renders at 512 pixels and up, the ZIP, the `.DS_Store` files, and an older copy of the package nested inside it
  are left out: `build.py` makes the renders and the ZIP again from the SVGs, and `brand/.gitignore` keeps them out
  when it does. The package's `SHA256SUMS` keeps only the lines for the files committed, unchanged, so
  `shasum -a 256 -c SHA256SUMS` still passes and records what was copied. `brand/README.md` says what the package
  is, which site files come from it, and how to regenerate them.
- **Proposed: the social image is the dark horizontal logo, 520 pixels wide, centered on the surface color
  (#F6F6F6)**, made by `brand/social.py` with `rsvg-convert` and Pillow, beside the package's `build.py`, so it can be
  regenerated when the brand changes. The logo is centered by what it draws rather than by its padding, keeps at
  least 340 pixels of margin on each side, and fits inside the middle 630-pixel square that some apps crop a wide
  image to. Its alt text becomes "Rulemart's logo", since the image no longer carries the tagline.
- **Proposed: the README's logo is 240 pixels wide, above the plain `# Rulemart` title**, with the README's text
  unchanged. The guide asks for the horizontal logo at 120 pixels or wider; the README is secondary (**Existing**), so
  the logo is left-aligned rather than a centered title block. It names the SVGs in `brand/logos/` by relative path,
  which GitHub renders.
- **Proposed: the Code Rules companion mark in `brand/code-rules/` stays in the package but is not used**, since the
  Code Rules site owns its own mark.

## Not in this slice

- Any change to layout, type, color tokens, or copy beyond the marks.
- The GitHub App's and OAuth app's icons, which are set in GitHub's settings, not in this repository. The guide's
  choice for them is the dark 512-pixel app tile, `icons/rulemart-app-dark-512.png`, which `python3 brand/build.py`
  renders.

## Verification

- `make check` and `make check-generated` pass; the icon tests cover the new SVG (no scripts, handlers, or
  external references) and a stylesheet that loads nothing.
- Before-and-after screenshots of the home page, a library page, and a rule page at 1280 and 390 pixels, light and
  dark, plus the favicon set in both themes and the social image, in the pull request.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
