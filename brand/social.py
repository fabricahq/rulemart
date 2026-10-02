"""Make the site's social image: the dark horizontal logo centered on the surface color, at 1200x630.

Requires Pillow and rsvg-convert. Run from anywhere: python3 brand/social.py
"""

from io import BytesIO
from pathlib import Path
import subprocess

from PIL import Image

ROOT = Path(__file__).resolve().parent
LOGO = ROOT / "logos/rulemart-horizontal-dark.svg"
OUTPUT = ROOT.parent / "internal/platform/web/static/social.png"

SIZE = (1200, 630)
SURFACE = "#f6f6f6"
# The logo's drawn width. It leaves a wide margin on every side and fits the middle square that some apps crop a
# wide image to.
LOGO_WIDTH = 520


def render(svg, width):
    """Render an SVG at a width, by rsvg-convert, as an RGBA image."""
    png = subprocess.run(["rsvg-convert", "-w", str(width), str(svg)], check=True, capture_output=True).stdout
    return Image.open(BytesIO(png)).convert("RGBA")


def drawn(image):
    """Crop an image to what is drawn on it, leaving out the logo's own transparent padding."""
    return image.crop(image.getchannel("A").getbbox())


def main():
    # Render big enough that the drawn part, without the logo's padding, is at least LOGO_WIDTH wide, then scale it
    # down to exactly that width, so the logo is centered by what is drawn rather than by its padding.
    logo = drawn(render(LOGO, LOGO_WIDTH * 2))
    logo = logo.resize((LOGO_WIDTH, round(logo.height * LOGO_WIDTH / logo.width)), Image.Resampling.LANCZOS)
    card = Image.new("RGB", SIZE, SURFACE)
    card.paste(logo, ((SIZE[0] - logo.width) // 2, (SIZE[1] - logo.height) // 2), logo)
    card.save(OUTPUT, optimize=True)


if __name__ == "__main__":
    main()
