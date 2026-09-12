"""Draw assets/dmg_background.png — what you see when the DMG opens.

    python scripts/make_dmg_background.py

Finder shows the background at one pixel per point, so the image is exactly the
Finder window `build_dmg.sh` asks for and the arrow is drawn between the two
icon centres that script places. Change either and run this again.
"""

from __future__ import annotations

import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

# Must match build_dmg.sh: --window-size, --icon and --app-drop-link.
WIDTH, HEIGHT = 600, 400
APP_X, DROP_X, ICON_Y = 150, 450, 200
ICON_SIZE = 128

SCALE = 4  # drawn large and downscaled, so the text and the arrow are smooth

BACKGROUND_TOP = (22, 22, 30)
BACKGROUND_BOTTOM = (11, 11, 16)
TITLE = (245, 245, 250)
SUBTITLE = (168, 168, 182)
HINT = (116, 116, 130)
ARROW = (96, 122, 214)

FONT_CANDIDATES = (
    "/System/Library/Fonts/SFNSRounded.ttf",
    "/System/Library/Fonts/SFNS.ttf",
    "/System/Library/Fonts/HelveticaNeue.ttc",
    "/System/Library/Fonts/Helvetica.ttc",
)

OUT = Path(__file__).resolve().parent.parent / "assets" / "dmg_background.png"


def font(size: int) -> ImageFont.ImageFont:
    for path in FONT_CANDIDATES:
        if Path(path).exists():
            try:
                return ImageFont.truetype(path, size * SCALE)
            except OSError:
                continue
    return ImageFont.load_default()


def centered(draw: ImageDraw.ImageDraw, y: int, text: str, f, fill) -> None:
    """`y` is the text's centre line, in points."""
    left, top, right, bottom = draw.textbbox((0, 0), text, font=f)
    draw.text(
        ((WIDTH * SCALE - (right - left)) / 2 - left, y * SCALE - (bottom - top) / 2 - top),
        text,
        font=f,
        fill=fill,
    )


def arrow(draw: ImageDraw.ImageDraw) -> None:
    """A shaft from the app icon to the Applications icon, clear of both."""
    gap = ICON_SIZE // 2 + 22
    x0, x1 = (APP_X + gap) * SCALE, (DROP_X - gap) * SCALE
    y = ICON_Y * SCALE
    head = 11 * SCALE

    draw.line((x0, y, x1 - head, y), fill=ARROW, width=2 * SCALE)
    draw.polygon(
        ((x1, y), (x1 - head, y - head * 0.62), (x1 - head, y + head * 0.62)),
        fill=ARROW,
    )


def gradient(draw: ImageDraw.ImageDraw) -> None:
    """Top-to-bottom wash.

    The two ends are eleven levels apart over four hundred points, which bands
    into visible stripes if each row is simply rounded. Rounding up or down by
    the row's position within each group of four instead spreads the step over
    four supersampled rows, and the downscale averages them into one clean
    intermediate value.
    """
    rows = HEIGHT * SCALE
    for row in range(rows):
        t = row / (rows - 1)
        threshold = (row % SCALE + 0.5) / SCALE
        colour = []
        for a, b in zip(BACKGROUND_TOP, BACKGROUND_BOTTOM):
            exact = a + (b - a) * t
            base = int(exact // 1)
            colour.append(base + 1 if exact - base > threshold else base)
        draw.line((0, row, WIDTH * SCALE, row), fill=tuple(colour))


def main() -> int:
    canvas = Image.new("RGB", (WIDTH * SCALE, HEIGHT * SCALE))
    draw = ImageDraw.Draw(canvas)

    gradient(draw)

    centered(draw, 58, "Clarity", font(30), TITLE)
    centered(draw, 92, "Drag Clarity into Applications", font(13), SUBTITLE)
    arrow(draw)
    centered(draw, 330, "First launch is blocked because Clarity isn't notarized:", font(11), HINT)
    centered(draw, 350, "System Settings → Privacy & Security → Open Anyway", font(11), HINT)
    centered(draw, 374, "Then grant Screen Recording and Input Monitoring — each needs a relaunch.", font(11), HINT)

    canvas.resize((WIDTH, HEIGHT), Image.Resampling.LANCZOS).save(OUT, format="PNG", optimize=True)
    print(f"wrote {OUT} ({WIDTH}x{HEIGHT})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
