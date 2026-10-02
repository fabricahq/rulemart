# Rulemart brand

Rulemart's brand package: the symbol (three stacked planes), the horizontal and stacked logos with an outlined Inter
Semibold wordmark, the app tiles, and the favicons, made on 2026-10-02. Open [guide.html](guide.html) for how to use
them: the colors, the clear space, and the smallest sizes.

`build.py` exports every file from [source/rulemart.svg](source/rulemart.svg) and the Inter font, and `SHA256SUMS`
records them. This directory keeps the SVGs and the PNGs up to 400 pixels wide; the larger renders and the package's
ZIP are left out, since `build.py` makes them again from the SVGs, so `SHA256SUMS` lists only the files kept here.
Run `shasum -a 256 -c SHA256SUMS` in this directory to check them.

## Where the site uses it

The site's `favicon.svg`, `favicon.ico`, and `apple-touch-icon.png` in `internal/platform/web/static/` are copied
from `favicons/` unchanged. The header draws the symbol's path from `source/rulemart.svg` inline, in the text's color,
with the heavier stroke `guide.html` gives the site's small sizes.

`static/social.png`, the image a link to Rulemart shows on social sites, is the dark horizontal logo centered on the
surface color, made by `social.py`. Run it after changing the logo, with Pillow and `rsvg-convert` installed:

```sh
python3 brand/social.py
```

## Regenerating the package

Install the packages in `requirements.txt` and `rsvg-convert`, then run `python3 brand/build.py`. It writes the large
renders and the ZIP too, which `.gitignore` keeps out of the repository, and rewrites `SHA256SUMS` over every file
in this directory; keep only the lines for committed files before committing it.
