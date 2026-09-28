#!/usr/bin/env python3
"""Draw a terminal capture as the README screenshot and the social preview.

The input is the output of `herdr pane read --format ansi` for a pane that
runs `make demo`. Herdr has already emulated the terminal, so every line is
the final screen row with SGR color sequences and no cursor movement. This
script only has to paint each cell at its grid position.

ANSI palette colors are drawn with Catppuccin Mocha, Herdr's default theme,
because the plugin uses palette indexes and the terminal theme decides them.

Usage: screenshot.py <capture.ansi> <out-dir> <name> [--social] [--cols N]
Writes <out-dir>/<name>.svg, and with --social also
<out-dir>/social-preview.svg. Convert them with rsvg-convert, as
`make screenshot` does. --cols sets the screen width. Pass the pane width, so
that every image of one run has the same size; without it the width is the
longest line.
"""

import html
import re
import sys

# Catppuccin Mocha: the 16 ANSI colors, then the default foreground and
# background, as Herdr's catppuccin theme uses them.
PALETTE = [
    "#45475a", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#bac2de",
    "#585b70", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#a6adc8",
]
FG, BG = "#cdd6f4", "#1e1e2e"
CRUST, MANTLE, SURFACE, SUBTEXT = "#11111b", "#181825", "#313244", "#a6adc8"

FONT = "'FiraCode Nerd Font Mono', 'Fira Code', Menlo, monospace"
CELL_W, CELL_H, FONT_SIZE = 9.0, 20.0, 15

SGR = re.compile(r"\x1b\[([0-9;]*)m")
# Every CSI sequence except SGR, which ends in "m" and carries the colors.
OTHER_ESC = re.compile(r"\x1b\[[0-9;?]*[A-Za-ln-z]|\r")


def xterm256(n):
    """The xterm 256-color cube and gray ramp, for any fixed color left over."""
    if n < 16:
        return PALETTE[n]
    if n < 232:
        n -= 16
        steps = [0, 95, 135, 175, 215, 255]
        return "#%02x%02x%02x" % (steps[n // 36], steps[(n // 6) % 6], steps[n % 6])
    v = 8 + 10 * (n - 232)
    return "#%02x%02x%02x" % (v, v, v)


def blend(a, b, t):
    """Mix color a toward b by t, which is how a faint cell reads on screen."""
    pa = [int(a[i:i + 2], 16) for i in (1, 3, 5)]
    pb = [int(b[i:i + 2], 16) for i in (1, 3, 5)]
    return "#%02x%02x%02x" % tuple(round(x + (y - x) * t) for x, y in zip(pa, pb))


def parse(line):
    """Split one captured row into cells of (char, fg, bg, bold, faint)."""
    cells = []
    fg = bg = None
    bold = faint = False
    pos = 0
    line = OTHER_ESC.sub("", line)
    for m in list(SGR.finditer(line)) + [None]:
        end = m.start() if m else len(line)
        for ch in line[pos:end]:
            cells.append((ch, fg, bg, bold, faint))
        if not m:
            break
        pos = m.end()
        params = [int(p) if p else 0 for p in m.group(1).split(";")]
        i = 0
        while i < len(params):
            p = params[i]
            if p == 0:
                fg = bg = None
                bold = faint = False
            elif p == 1:
                bold = True
            elif p == 2:
                faint = True
            elif p == 22:
                bold = faint = False
            elif 30 <= p <= 37:
                fg = PALETTE[p - 30]
            elif 90 <= p <= 97:
                fg = PALETTE[p - 90 + 8]
            elif p == 39:
                fg = None
            elif 40 <= p <= 47:
                bg = PALETTE[p - 40]
            elif p == 49:
                bg = None
            elif p in (38, 48) and i + 1 < len(params):
                if params[i + 1] == 5 and i + 2 < len(params):
                    color = xterm256(params[i + 2])
                    i += 2
                elif params[i + 1] == 2 and i + 4 < len(params):
                    color = "#%02x%02x%02x" % tuple(params[i + 2:i + 5])
                    i += 4
                else:
                    color = None
                if p == 38:
                    fg = color
                else:
                    bg = color
            i += 1
    return cells


def crop(rows):
    """Keep the header through the last task, then the dots and the footer.

    The UI pads the list with blank rows to fill the pane. A screenshot does
    not need them, so they are dropped and the last two content rows (the
    page dots, when there are any, and the key hints) follow directly.
    """
    text = ["".join(c[0] for c in r).rstrip() for r in rows]
    content = [i for i, t in enumerate(text) if t]
    if not content:
        return rows
    last_task = max(i for i in content if i < content[-1])
    return rows[:last_task + 1] + [[]] + [rows[content[-1]]]


def terminal(rows, cols, x0, y0):
    """The SVG elements that paint the rows inside a window at x0, y0."""
    out = []
    for r, cells in enumerate(rows):
        # The capture comes back a few cells short of the pane on a row that
        # ends in colored blanks, but the UI writes the selection background
        # across the full width (a UI test asserts it). Extend such a row, so
        # the highlight reaches the edge as it does on screen.
        if cells and cells[-1][2] and cells[-1][0] == " ":
            cells = cells + [cells[-1]] * (cols - len(cells))
        y = y0 + r * CELL_H
        # Backgrounds first, in runs, so the text sits on top.
        c = 0
        while c < min(len(cells), cols):
            bg = cells[c][2]
            start = c
            while c < min(len(cells), cols) and cells[c][2] == bg:
                c += 1
            if bg:
                out.append('<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>'
                           % (x0 + start * CELL_W, y, (c - start) * CELL_W, CELL_H, bg))
        # Text in runs of one style, one element for each character, so every
        # glyph sits on its cell whatever the font's advance is. rsvg honors
        # neither textLength with trailing spaces nor a list of x positions,
        # so a single element for the run drifts off the grid.
        c = 0
        while c < min(len(cells), cols):
            _, fg, _, bold, faint = cells[c]
            start = c
            while c < min(len(cells), cols) and cells[c][1] == fg and cells[c][3:] == (bold, faint):
                c += 1
            run = "".join(ch for ch, *_ in cells[start:c])
            if not run.strip():
                continue
            color = fg or FG
            if faint:
                color = blend(color, BG, 0.45)
            glyphs = "".join(
                '<text x="%.1f">%s</text>' % (x0 + (start + i) * CELL_W, html.escape(ch))
                for i, ch in enumerate(run) if ch != " ")
            out.append('<g transform="translate(0 %.1f)"%s fill="%s">%s</g>'
                       % (y + CELL_H * 0.72, ' font-weight="bold"' if bold else "", color, glyphs))
    return out


def window(rows, cols, x, y, title):
    """A terminal window: title bar, traffic lights and the screen."""
    pad = 16
    w = cols * CELL_W + 2 * pad
    h = len(rows) * CELL_H + 2 * pad + 32
    parts = [
        '<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="12" fill="%s" stroke="%s"/>' % (x, y, w, h, BG, SURFACE),
        '<path d="M%.1f %.1fh%.1fv20h-%.1fz" fill="%s"/>' % (x + 1, y + 12, w - 2, w - 2, MANTLE),
        '<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="11" fill="%s"/>' % (x + 1, y + 1, w - 2, 31, MANTLE),
    ]
    for i, color in enumerate(("#f38ba8", "#f9e2af", "#a6e3a1")):
        parts.append('<circle cx="%.1f" cy="%.1f" r="6" fill="%s"/>' % (x + 20 + i * 20, y + 16, color))
    parts.append('<text x="%.1f" y="%.1f" text-anchor="middle" fill="%s" font-size="13">%s</text>'
                 % (x + w / 2, y + 21, SUBTEXT, html.escape(title)))
    parts += terminal(rows, cols, x + pad, y + 32 + pad)
    return parts, w, h


def svg(width, height, body, bg=None):
    head = ('<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" '
            'font-family="%s" font-size="%d" xml:space="preserve">' % (width, height, width, height, FONT, FONT_SIZE))
    fill = ['<rect width="100%%" height="100%%" fill="%s"/>' % bg] if bg else []
    return "\n".join([head] + fill + body + ["</svg>"]) + "\n"


def main():
    capture, out, name = sys.argv[1], sys.argv[2], sys.argv[3]
    flags = sys.argv[4:]
    social = "--social" in flags
    with open(capture, encoding="utf-8") as f:
        rows = crop([parse(line.rstrip("\n")) for line in f])
    cols = max(len("".join(c[0] for c in r).rstrip()) for r in rows)
    if "--cols" in flags:
        cols = int(flags[flags.index("--cols") + 1])

    # A README screenshot: the window alone on a transparent canvas.
    parts, w, h = window(rows, cols, 1, 1, "herdr · dstask")
    with open(out + "/" + name + ".svg", "w", encoding="utf-8") as f:
        f.write(svg(int(w) + 2, int(h) + 2, parts))
    if not social:
        return

    # The social preview: GitHub's 1280x640, with the name above the window.
    W, H = 1280, 640
    parts, w, h = window(rows, cols, 0, 0, "herdr · dstask")
    scale = min((W - 120) / w, (H - 200) / h)
    # Center the window in the space under the subtitle.
    top = 160
    x, y = (W - w * scale) / 2, top + (H - top - h * scale) / 2
    body = [
        '<text x="%d" y="86" text-anchor="middle" fill="%s" font-size="54" font-weight="bold">herdr-dstask</text>' % (W / 2, FG),
        '<text x="%d" y="128" text-anchor="middle" fill="%s" font-size="22">dstask in the Herdr control plane</text>' % (W / 2, SUBTEXT),
        '<g transform="translate(%.1f %.1f) scale(%.4f)">' % (x, y, scale),
    ] + parts + ["</g>"]
    with open(out + "/social-preview.svg", "w", encoding="utf-8") as f:
        f.write(svg(W, H, body, bg=CRUST))


if __name__ == "__main__":
    main()
