from __future__ import annotations

import sys
from pathlib import Path

import av
from PIL import Image, ImageDraw, ImageFont


VIDEO = Path(sys.argv[1])
OUTPUT = Path(sys.argv[2])
OUTPUT.mkdir(parents=True, exist_ok=True)

FONT_PATH = "/System/Library/Fonts/Supplemental/Arial.ttf"
FONT = ImageFont.truetype(FONT_PATH, 24)
SMALL_FONT = ImageFont.truetype(FONT_PATH, 19)


def read_frames(frame_numbers: set[int]) -> dict[int, Image.Image]:
    container = av.open(str(VIDEO))
    images: dict[int, Image.Image] = {}
    for index, frame in enumerate(container.decode(video=0)):
        if index in frame_numbers:
            images[index] = frame.to_image()
        if len(images) == len(frame_numbers):
            break
    return images


def tile(image: Image.Image, label: str, size: tuple[int, int]) -> Image.Image:
    width, height = size
    canvas = Image.new("RGB", (width, height + 42), "#13052f")
    fitted = image.copy()
    fitted.thumbnail((width, height), Image.Resampling.LANCZOS)
    canvas.paste(fitted, ((width - fitted.width) // 2, 0))
    draw = ImageDraw.Draw(canvas)
    draw.text((14, height + 8), label, font=SMALL_FONT, fill="#fffdf7")
    return canvas


def sheet(
    entries: list[tuple[int, str]],
    images: dict[int, Image.Image],
    columns: int,
    output_name: str,
) -> None:
    tile_width, tile_height = 480, 270
    rows = (len(entries) + columns - 1) // columns
    canvas = Image.new(
        "RGB",
        (columns * tile_width, rows * (tile_height + 42)),
        "#13052f",
    )
    for index, (frame, label) in enumerate(entries):
        item = tile(images[frame], f"{label} · f{frame}", (tile_width, tile_height))
        x = (index % columns) * tile_width
        y = (index // columns) * (tile_height + 42)
        canvas.paste(item, (x, y))
    canvas.save(OUTPUT / output_name, quality=92)


overview = [
    (30, "Open"),
    (90, "Create project"),
    (160, "Create exit"),
    (220, "Deploy command"),
    (286, "Release staged"),
    (345, "Upload"),
    (431, "Release verified"),
    (500, "Protected reveal"),
    (580, "Live URL"),
    (625, "Gateway authorization"),
    (680, "Brand reveal"),
    (710, "End card"),
]

transitions = [
    (173, "Create before snap"),
    (174, "Deploy after snap"),
    (318, "Deploy before snap"),
    (319, "Upload after snap"),
    (493, "Upload hidden cut"),
    (494, "Protected hidden cut"),
    (638, "Protected handoff"),
    (639, "Brand handoff"),
    (655, "Lock compress"),
    (656, "Cloud morph"),
]

requested = {frame for frame, _ in overview + transitions}
frames = read_frames(requested)
missing = sorted(requested - frames.keys())
if missing:
    raise RuntimeError(f"Missing frames: {missing}")

sheet(overview, frames, 4, "final-contact-sheet.jpg")
sheet(transitions, frames, 2, "transition-proof-sheet.jpg")
frames[710].save(OUTPUT / "end-card.png")
