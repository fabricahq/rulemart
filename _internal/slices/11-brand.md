# Slice R2: brand

## Goal

Give Rulemart its own mark, from the brand package in `brand/`, everywhere the site shows one: the home page's
hero, the browser tab, a phone's home screen, links shared on social sites, and the README. One pull request,
reviewed from its before-and-after screenshots, so the look is judged on its own.
[realignment.md](../realignment.md) explains why it is a slice of its own.

Decisions marked **Proposed** are new in this slice and wait for review. **Decided** ones are Josh's. **Existing**
ones describe what the site already does.

## What a visitor sees

- **The header** is unchanged: Fabrica's cube and name, a slash, then "Rulemart" in text, as Code Rules' header reads
  "Fabrica / Code Rules".
- **The home page's hero** opens with Rulemart's horizontal logo, symbol and wordmark, centered where the "Fabrica /
  Rulemart" eyebrow was, as Code Rules' home page opens with its own: the ink logo in the light theme and the white
  one in the dark, 56 pixels tall, 48 on a phone. The heading, lede, search, and Popular chips below it are unchanged.
- **The browser tab** shows the package's adaptive favicon, whose stroke is dark in the browser's light theme and
  light in its dark theme, with an ICO of the package's dark favicon artwork for browsers that take no SVG.
- **A phone's home screen** shows the package's apple-touch-icon, the symbol on a dark tile.
- **A link shared** on a social site shows a 1200x630 image of the dark horizontal logo, symbol and wordmark, over
  the tagline "Agent coding best practices, off the shelf", centered on the surface color.
- **The README** opens with the horizontal logo, dark or white with the reader's theme, through a `<picture>`
  element.

## Decisions

- **Decided (Josh): the header keeps its brand as text, and the logo goes in the home page's hero**, modeled on
  [Code Rules' site](https://code-rules.fabricahq.com/): the header reads Fabrica's cube, "Fabrica", a slash, and
  "Rulemart", exactly as before this slice, and the hero's eyebrow becomes the horizontal logo lockup, centered, 56
  pixels tall at desktop and 48 on phones.
- **Proposed: the hero shows two `<img>` elements, one per theme**, `rulemart-horizontal-dark.svg` and
  `rulemart-horizontal-white.svg`, copied unchanged from `brand/logos/` into the static files and served under their
  hashed names, each with its width and height and `alt="Rulemart"`. A new `dark:` variant in the stylesheet shows
  one and hides the other by the same rule as the color tokens: dark when the visitor chose dark in the footer, or
  chose nothing and their system prefers dark. A `<picture>` with a `prefers-color-scheme` source would ignore the
  footer's choice. The hidden image isn't rendered, so screen readers read Rulemart once.
- **Proposed: `favicon.svg` and `apple-touch-icon.png` are the package's files, copied unchanged** into
  `internal/platform/web/static/` under the names the pages and the `/favicon.ico` route already use (**Existing**).
  Static files are served under hashed names and cached for a year, so replacing them needs no infrastructure change
  (**Existing**); `/favicon.ico` is cached a day.
- **Proposed: `favicon.ico` is built from the package's dedicated favicon artwork, not copied.** The package's ICO
  scales its 256-pixel app tile, the symbol on #202020, down to each size, which leaves a gray smudge at 16 pixels
  and looks nothing like the SVG favicon. `brand/assets.py` instead puts `favicons/rulemart-dark-16.png`, `-32.png`,
  and `-48.png`, drawn for those sizes with the heavier stroke, into the ICO unchanged, one frame a size. Like
  those files, its stroke is the ink color on transparency, so a browser that shows the ICO in a dark tab strip
  shows a dark mark on dark; every current browser takes the adaptive SVG instead.
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
- **Decided (Josh): the social image carries the tagline.** It is the dark horizontal logo, 520 pixels wide, over
  "Agent coding best practices, off the shelf" in Inter Regular at 34 pixels in the ink color (#1C1C1C), 36 pixels
  below it, the two centered as one block on the surface color (#F6F6F6). The logo stays dominant, keeps 340 pixels
  of margin on each side, and fits inside the middle 630-pixel square that some apps crop a wide image to; the
  tagline, 684 pixels wide, is clipped at its ends by such a crop. The alt text is "Rulemart: agent coding best
  practices, off the shelf", as it was.
- **Proposed: `brand/assets.py` makes the social image and `favicon.ico`**, beside the package's `build.py`, with `rsvg-convert` for
  the logo and Pillow for the tagline, which reads the package's `source/inter-latin.woff2` directly and sets its
  Regular instance, so it can be regenerated when the brand changes and gives the same bytes on every run. It
  centers the logo by what it draws rather than its padding, and the tagline by its capitals and baseline.
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
