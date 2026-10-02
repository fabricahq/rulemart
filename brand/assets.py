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
LOGO = ROOT / "logos/rulemart-horizontal-dark.svg"
STATIC = ROOT.parent / "internal/platform/web/static"

SIZE = (1200, 630)
SURFACE = "#f6f6f6"
INK = "#1c1c1c"
TAGLINE = "Agent coding best practices, off the shelf"
# The package's Inter, the variable font the site serves. Pillow reads the WOFF2 itself and sets its weight by the
# named instance, so no converted copy is needed.
FONT = ROOT / "source/inter-latin.woff2"
TAGLINE_SIZE = 34
# The space between the logo's drawing and the tagline's capitals.
GAP = 36
# The logo's drawn width. It leaves a wide margin on every side and fits the middle square that some apps crop a
# wide image to.
LOGO_WIDTH = 520

FAVICON_SIZES = (16, 32, 48)


def render(svg, width):
    """Render an SVG at a width, by rsvg-convert, as an RGBA image."""
    png = subprocess.run(["rsvg-convert", "-w", str(width), str(svg)], check=True, capture_output=True).stdout
    return Image.open(BytesIO(png)).convert("RGBA")


def drawn(image):
    """Crop an image to what is drawn on it, leaving out the logo's own transparent padding."""
    return image.crop(image.getchannel("A").getbbox())


def social_image():
    # Render big enough that the drawn part, without the logo's padding, is at least LOGO_WIDTH wide, then scale it
    # down to exactly that width, so the logo is centered by what is drawn rather than by its padding.
    logo = drawn(render(LOGO, LOGO_WIDTH * 2))
    logo = logo.resize((LOGO_WIDTH, round(logo.height * LOGO_WIDTH / logo.width)), Image.Resampling.LANCZOS)
    font = ImageFont.truetype(FONT, TAGLINE_SIZE)
    font.set_variation_by_name("Regular")
    # The tagline's box from its capitals' top to its baseline, so the gap and the centering ignore the descenders'
    # and accents' room, as the eye does.
    left, _, right, _ = font.getbbox(TAGLINE, anchor="ls")
    cap = -font.getbbox("H", anchor="ls")[1]
    block = logo.height + GAP + cap
    y = (SIZE[1] - block) // 2

    card = Image.new("RGB", SIZE, SURFACE)
    card.paste(logo, ((SIZE[0] - logo.width) // 2, y), logo)
    baseline = y + logo.height + GAP + cap
    x = (SIZE[0] - (right - left)) // 2 - left
    ImageDraw.Draw(card).text((x, baseline), TAGLINE, font=font, fill=INK, anchor="ls")
    card.save(STATIC / "social.png", optimize=True)


def favicon_ico():
    frames = [Image.open(ROOT / f"favicons/rulemart-dark-{size}.png") for size in FAVICON_SIZES]
    largest = frames[-1]
    largest.save(STATIC / "favicon.ico", sizes=[f.size for f in frames], append_images=frames[:-1])


if __name__ == "__main__":
    social_image()
    favicon_ico()
