"""Make the site's files that the package doesn't ship as the site needs them: the social image and favicon.ico.

The social image is the dark horizontal logo over the tagline, centered on the surface color, at 1200x630.
favicon.ico holds the package's dedicated favicon artwork, favicons/rulemart-dark-{16,32,48}.png, one frame a size:
the package's own ICO scales its 256-pixel app tile down, which leaves a gray smudge at 16 pixels.

Requires Pillow and rsvg-convert. Run from anywhere: python3 brand/assets.py
Both outputs are the same bytes on every run with the same Pillow and rsvg-convert.
"""

from io import BytesIO
from pathlib import Path
import subprocess

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parent
STATIC = ROOT.parent / "internal/platform/web/static"

LOGO = ROOT / "logos/rulemart-horizontal-dark.svg"
SIZE = (1200, 630)
SURFACE = "#f6f6f6"
# The guide's muted gray, so the tagline reads below the logo rather than beside it.
MUTED = "#626262"
# The tagline in the hero's two lines.
TAGLINE = ("Agent coding best practices,", "off the shelf")
# The package's Inter, the variable font the site serves. Pillow reads the WOFF2 itself and sets its weight by the
# named instance, so no converted copy is needed.
FONT = ROOT / "source/inter-latin.woff2"
TAGLINE_SIZE = 38
LINE_HEIGHT = 1.3
# The logo's drawn width. The logo and the tagline, which is no wider, stay inside the middle 630-pixel square that
# some apps crop a wide image to.
LOGO_WIDTH = 520

FAVICON_SIZES = (16, 32, 48)


def render(svg, width):
    """Render an SVG at a width, by rsvg-convert, as an RGBA image."""
    png = subprocess.run(["rsvg-convert", "-w", str(width), str(svg)], check=True, capture_output=True).stdout
    return Image.open(BytesIO(png)).convert("RGBA")


def drawn(image):
    """Crop an image to what is drawn on it, leaving out the logo's own transparent padding."""
    return image.crop(image.getchannel("A").getbbox())


def widest_gap(image):
    """Return the widest run of empty columns inside an image cropped to its drawing: in the logo, the space between
    the symbol and the wordmark."""
    alpha = image.getchannel("A")
    empty = [alpha.crop((x, 0, x + 1, image.height)).getbbox() is None for x in range(image.width)]
    widest = run = 0
    for column in empty:
        run = run + 1 if column else 0
        widest = max(widest, run)
    return widest


def social_image():
    # Render big enough that the drawing, without the logo's padding, is at least LOGO_WIDTH wide, then scale it down
    # to exactly that width, so the logo is centered by what it draws rather than by its padding.
    logo = drawn(render(LOGO, LOGO_WIDTH * 2))
    logo = logo.resize((LOGO_WIDTH, round(logo.height * LOGO_WIDTH / logo.width)), Image.Resampling.LANCZOS)
    # The tagline sits as far below the logo as the wordmark sits from the symbol, so the logo stays one unit.
    gap = widest_gap(logo)

    font = ImageFont.truetype(FONT, TAGLINE_SIZE)
    font.set_variation_by_name("Regular")
    # Lines are measured from the capitals' top to the baseline, so the gap and the centering ignore the descenders'
    # room, as the eye does.
    cap = -font.getbbox("H", anchor="ls")[1]
    step = round(TAGLINE_SIZE * LINE_HEIGHT)
    lines = [(line, font.getbbox(line, anchor="ls")) for line in TAGLINE]
    for line, (left, _, right, _) in lines:
        if right - left > LOGO_WIDTH:
            raise SystemExit(f"the tagline's line {line!r} is {right - left} pixels, wider than the logo")
    block = logo.height + gap + cap + step * (len(lines) - 1)
    top = (SIZE[1] - block) // 2

    card = Image.new("RGB", SIZE, SURFACE)
    card.paste(logo, ((SIZE[0] - logo.width) // 2, top), logo)
    draw = ImageDraw.Draw(card)
    baseline = top + logo.height + gap + cap
    for line, (left, _, right, _) in lines:
        draw.text(((SIZE[0] - (right - left)) // 2 - left, baseline), line, font=font, fill=MUTED, anchor="ls")
        baseline += step
    card.save(STATIC / "social.png", optimize=True)


def favicon_ico():
    frames = [Image.open(ROOT / f"favicons/rulemart-dark-{size}.png") for size in FAVICON_SIZES]
    largest = frames[-1]
    largest.save(STATIC / "favicon.ico", sizes=[f.size for f in frames], append_images=frames[:-1])


if __name__ == "__main__":
    social_image()
    favicon_ico()
