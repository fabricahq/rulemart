# Rulemart brand

Rulemart's brand package: the symbol (three stacked planes), the horizontal and stacked logos with an outlined Inter
Semibold wordmark, the app tiles, and the favicons, made on 2026-10-02. Open [guide.html](guide.html) for how to use
them: the colors, the clear space, and the smallest sizes.

`build.py` exports every file from [source/rulemart.svg](source/rulemart.svg) and the Inter font, and `SHA256SUMS`
records them. This directory keeps the SVGs and the PNGs up to 400 pixels wide; the larger renders and the package's
ZIP are left out, since `build.py` makes them again from the SVGs, so `SHA256SUMS` lists only the files kept here.
Run `shasum -a 256 -c SHA256SUMS` in this directory to check them.

## Where the site uses it

The site's `favicon.svg` and `apple-touch-icon.png` in `internal/platform/web/static/` are copied from `favicons/`
unchanged, and so are `rulemart-horizontal-dark.svg` and `rulemart-horizontal-white.svg` from `logos/`, which the
home page's hero shows in the light and dark themes. The repository's README shows the same logos, dark or white
with the reader's theme.

`assets.py` makes the two site files the package doesn't ship as the site needs them:

- `static/favicon.ico` holds `favicons/rulemart-dark-16.png`, `-32.png`, and `-48.png`, one frame a size. It
  departs from the package's own `favicons/favicon.ico`, which scales the 256-pixel app tile down and blurs at 16
  pixels.
- `static/social.png`, the image a link to Rulemart shows on social sites, is the dark horizontal logo over the
  tagline, centered on the surface color.

Run it after changing the logo, the favicon artwork, or the tagline, with Pillow and `rsvg-convert` installed:

```sh
python3 brand/assets.py
```

## Regenerating the package

Install the packages in `requirements.txt` and `rsvg-convert`, then run `python3 brand/build.py`. It writes the large
renders and the ZIP too, which `.gitignore` keeps out of the repository, and rewrites `SHA256SUMS` over every file
in this directory; keep only the lines for committed files before committing it.

`guide.html` differs from the package in one line: its package download link leads to this directory, since the repository holds no ZIP. `SHA256SUMS` records the checksum of that edited file.
