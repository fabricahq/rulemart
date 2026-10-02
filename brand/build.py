"""Export the Rulemart brand assets. Requires fonttools[woff], uharfbuzz, Pillow, and rsvg-convert."""

from io import BytesIO
from pathlib import Path
import hashlib
import subprocess
import zipfile
import xml.etree.ElementTree as ET

from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from PIL import Image
import uharfbuzz as hb


ROOT = Path(__file__).resolve().parent
MARK = ET.parse(ROOT / "source/rulemart.svg").find(".//{http://www.w3.org/2000/svg}path").get("d")


def symbol(color, width=3, transform=""):
    """Render the approved paths at a chosen optical weight and scale."""
    return f'<path d="{MARK}" fill="none" stroke="{color}" stroke-width="{width}" stroke-linecap="round" stroke-linejoin="round" transform="{transform}"/>'


def document(content, width=64, height=64):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">{content}</svg>\n'


def export_svg(name, content, sizes):
    """Save an SVG and render each requested PNG width directly from the vector."""
    svg = ROOT / f"{name}.svg"
    svg.parent.mkdir(parents=True, exist_ok=True)
    svg.write_text(content)
    for size in sizes:
        subprocess.run(["rsvg-convert", "-w", str(size), str(svg), "-o", str(ROOT / f"{name}-{size}.png")], check=True)


def wordmark():
    """Outline Inter Semibold with real font shaping, returning paths and their advance."""
    font = instantiateVariableFont(TTFont(ROOT / "source/inter-latin.woff2"), {"wght": 600})
    font.flavor = None
    data = BytesIO()
    font.save(data)
    shaped_font = hb.Font(hb.Face(data.getvalue()))
    buffer = hb.Buffer()
    buffer.add_str("Rulemart")
    buffer.guess_segment_properties()
    hb.shape(shaped_font, buffer)
    glyphs = font.getGlyphSet()
    order = font.getGlyphOrder()
    scale = 48 / font["head"].unitsPerEm
    x = 0
    paths = []
    for info, position in zip(buffer.glyph_infos, buffer.glyph_positions):
        pen = SVGPathPen(glyphs)
        glyphs[order[info.codepoint]].draw(pen)
        paths.append(f'<path transform="translate({x + position.x_offset * scale:.4f} {-position.y_offset * scale:.4f}) scale({scale} {-scale})" d="{pen.getCommands()}"/>')
        x += position.x_advance * scale - 1.2
    return ''.join(paths), x + 1.2


def export_logos():
    outlined, advance = wordmark()
    for tone, color in [("dark", "#1c1c1c"), ("white", "#ffffff")]:
        export_svg(f"logos/rulemart-symbol-{tone}", document(symbol(color)), [1024, 512, 256])
        width = round(96 + advance + 24)
        lockup = symbol(color, transform="translate(12 16)") + f'<g fill="{color}" transform="translate(90 66)">{outlined}</g>'
        export_svg(f"logos/rulemart-horizontal-{tone}", document(lockup, width, 96), [1600, 800, 400])
        stacked = symbol(color, transform="translate(80 8) scale(1.5)") + f'<g fill="{color}" transform="translate({(256-advance)/2:.4f} 142)">{outlined}</g>'
        export_svg(f"logos/rulemart-stacked-{tone}", document(stacked, 256, 176), [1024, 512])


def export_icons():
    for tone, ink, paper in [("dark", "#ffffff", "#202020"), ("light", "#1c1c1c", "#ffffff")]:
        content = f'<rect width="64" height="64" fill="{paper}"/>' + symbol(ink, 3.5, "translate(8 8) scale(.75)")
        export_svg(f"icons/rulemart-app-{tone}", document(content), [1024, 512, 256, 128, 64, 32, 16])
    for tone, color in [("dark", "#1c1c1c"), ("white", "#ffffff")]:
        export_svg(f"favicons/rulemart-{tone}", document(symbol(color, 4)), [16, 32, 48])
    style = '<style>path{stroke:#1c1c1c}@media(prefers-color-scheme:dark){path{stroke:#f2f2f2}}</style>'
    export_svg("favicons/favicon", document(style + symbol("#1c1c1c", 4)), [])
    image = Image.open(ROOT / "icons/rulemart-app-dark-256.png")
    image.save(ROOT / "favicons/favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)])
    subprocess.run(["rsvg-convert", "-w", "180", str(ROOT / "icons/rulemart-app-dark.svg"), "-o", str(ROOT / "favicons/apple-touch-icon.png")], check=True)


def package():
    """Package all deliverables with checksums, excluding the archive itself."""
    files = sorted(p for p in ROOT.rglob('*') if p.is_file() and p.name not in ["Rulemart-Brand-Package.zip", "SHA256SUMS"])
    manifest = ROOT / "SHA256SUMS"
    manifest.write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.relative_to(ROOT)}\n' for p in files))
    with zipfile.ZipFile(ROOT / "Rulemart-Brand-Package.zip", "w", zipfile.ZIP_DEFLATED) as archive:
        for path in files + [manifest]:
            archive.write(path, Path("Rulemart-Brand-Package") / path.relative_to(ROOT))


if __name__ == "__main__":
    export_logos()
    export_icons()
    package()
