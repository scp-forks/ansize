# Ansize

Converts images to text-mode artwork, in three flavors:

- **`bbs`** (default) — authentic 1980s BBS-scene ANSI art: the 16-color
  CGA palette, CP437 block characters (`░ ▒ ▓ █ ▀ ▄`), ANSI.SYS-compatible
  escape codes, CRLF line endings, the scene's 79-column rule, and a
  [SAUCE](https://www.acid.org/info/sauce/sauce.htm) metadata record.
  The resulting `.ans` files render correctly in period viewers and modern
  renderers like [ansilove](https://www.ansilove.org/) and
  [PabloDraw](https://picoe.ca/products/pablodraw/), and would have worked
  on a real DOS BBS over a 2400-baud modem.
- **`ascii`** — 1970s-style monochrome art: pure 7-bit ASCII shaded with
  Paul Bourke's density ramp (`@%#*+=-:. `), the way terminal and
  line-printer art was made before color existed.
- **`modern`** — the original ansize behavior: 256-color xterm codes with
  random `0`/`1` characters.

Check out the examples folder for image samples and their corresponding
output (`.ans` = bbs, `.asc` = ascii, `.ansi` = modern).

## Usage

    ansize [options] <image> <output> [width]   # convert an image
    ansize [options] <art.ans|art.asc>          # view a CP437 ANSI/ASCII file

A live preview is printed to the terminal while the file is written.

    ansize photo.png art.ans                    # 79-column ANSI with SAUCE record
    ansize -ice -author "you" in.png out.ans    # iCE colors, credited
    ansize -mode ascii in.png out.asc           # 1970s ASCII
    ansize -mode modern in.png out.ansi 100     # original behavior

Options:

    -mode string     "bbs", "ascii", or "modern" (default "bbs")
    -width int       output width in columns (default 79 bbs/ascii, 100 modern;
                     the trailing positional argument also works)
    -ice             bbs: iCE colors -- unlocks all 16 background colors by
                     reinterpreting the blink attribute as bright background
                     (sets the matching SAUCE flag)
    -dither          bbs: ordered Bayer 4x4 dithering (default true)
    -no-sauce        bbs: skip the SAUCE metadata record
    -title string    SAUCE title (default: output file name)
    -author string   SAUCE author
    -group string    SAUCE group
    -invert          ascii: dense glyphs for dark pixels (for printing on white)
    -charset string  modern: characters to draw with (default "01")
    -baud int        view: pace playback like a modem (300, 1200, 2400...; 0 = instant)
    -16              view: terminal's own 16 colors instead of exact CGA truecolor

## Viewing .ans files

`.ans` files are raw CP437 bytes, like the originals — a UTF-8 terminal
will show mojibake if you just `cat` them. ansize doubles as a viewer:
give it a single `.ans`/`.asc` file (any CP437 ANSI works, including
scene art from [16colo.rs](https://16colo.rs)) and it replays the file
the way ANSI.SYS would have — CP437 decoded to the right glyphs, the
exact IBM 5153 CGA colors via truecolor escapes, SAUCE respected (wrap
width, iCE colors), cursor sequences honored, rendering stopped at the
DOS EOF byte:

    ansize art.ans               # view instantly
    ansize -baud 2400 art.ans    # watch it arrive like it's 1989
    ansize -16 art.ans           # terminals without 24-bit color

`-baud` paces playback like a serial line (ten line bits per byte, so
2400 baud is the authentic 240 characters per second). For pixel-perfect
renders with a real VGA font:

    ansilove art.ans            # render to PNG (brew install ansilove)

or open them in PabloDraw / [Moebius](https://blocktronics.github.io/moebius/).

## How it works

BBS mode samples two vertical points per character cell (a DOS 8x16 cell
is twice as tall as wide) and brute-forces the best (glyph, foreground,
background) triple over half blocks `▀ ▄` — two pixels per cell — and the
shade blocks `░ ▒ ▓` , which mix foreground over background at 25/50/75%
coverage. Candidates are scored with the
[redmean](https://www.compuphase.com/cmetric.htm) color distance, shade
mixes are computed in linear light, and a penalty keeps it from "mixing"
clashing colors that read as speckle at cell size. Output emits only
escape codes MS-DOS ANSI.SYS understood (`ESC[0m`, `ESC[1m`, `ESC[5m`,
`ESC[30-37m`, `ESC[40-47m`) — bright foregrounds are bold, and bright
backgrounds (with `-ice`) are blink, exactly as the scene did it.

## Transparency

ANSI art has no alpha channel. The screen starts black, and the only
"nothing" the format can express is a cell the file never paints — on a
real BBS, the background around a piece literally was the terminal
screen showing through. ansize maps PNG/GIF transparency onto exactly
that: transparent pixels read as black, black areas become unpainted
space cells, and each row is trimmed at its last visible cell, so
transparent regions cost no bytes at all (it also kept files small over
a modem). A moon on a transparent background comes out as just the moon,
floating on the bare screen.

The flip side: transparency is always composited *over black*, including
semi-transparent edge pixels, which blend toward black. Art drawn to sit
on a light page will come out dark — flatten it onto its intended
background first:

    magick in.png -background white -flatten flat.png

Note the output paints interior transparent areas as real spaces (black
holes if layered over other screen content), not the cursor-skip
sequences BBS overlays like menus and lightbars used.

## Installation

    go install github.com/jhchen/ansize@latest

## Development

1. Install go
2. Clone ansize
3. `go build`
