# godraw

Excalidraw-ish infinite canvas for Wayland/Hyprland, written in Go.

Hand-drawn feel via [perfect-freehand](https://github.com/steveruizok/perfect-freehand). Transparent overlay mode lets you draw directly over your desktop — think Gromit-MPX but with shapes, zoom, pan, and undo.

![godraw demo](https://raw.githubusercontent.com/GabriWar/godraw/main/demo.gif)

---

## Features

- **Freehand pen** with pressure-simulated variable-width strokes
- **Shapes**: rectangle, ellipse, line, arrow — all with the same hand-drawn look
- **Transparent overlay mode** — draws over your desktop, no background
- **Infinite canvas** — pan and zoom freely
- **Undo / clear**
- **4 colors, 3 brush sizes**
- Pure Go + [Gio](https://gioui.org) — no Electron, no browser, no JS

---

## Install

```bash
git clone https://github.com/GabriWar/godraw
cd godraw
go build -o godraw .
```

Requires Go 1.21+ and a Wayland compositor.

---

## Usage

```bash
./godraw              # normal canvas mode
./godraw -overlay     # transparent overlay over your desktop
```

### Keybinds

| Key | Action |
|-----|--------|
| `P` | Pen tool |
| `R` | Rectangle |
| `O` | Oval / ellipse |
| `L` | Line |
| `A` | Arrow |
| `1` `2` `3` `4` | Switch color |
| `S` | Cycle brush size |
| `C` | Clear canvas |
| `Ctrl+Z` | Undo last stroke |
| `0` | Reset zoom/pan |
| Left drag | Draw / shape |
| Middle/Right drag | Pan |
| Scroll | Zoom |

---

## Hyprland setup

Add to `windowsworkspaces.conf`:

```ini
windowrule = float on, match:class ^(godraw)$
windowrule = no_blur on, match:class ^(godraw)$
windowrule = opacity 1, match:class ^(godraw)$
windowrule = no_shadow on, match:class ^(godraw)$
windowrule = move cursor -50% -50%, match:class ^(godraw)$
```

Add to `keybinds.conf`:

```ini
bind = $mainMod, U, exec, /path/to/godraw -overlay
```

---

## Stack

- [Gio](https://gioui.org) — GPU-accelerated immediate-mode UI, native Wayland
- [perfect-freehand](https://github.com/steveruizok/perfect-freehand) — ported to Go for hand-drawn stroke feel
- Vendored Gio patch for per-pixel transparency (`GIO_TRANSPARENT` env var strips the opaque-region hint so Hyprland composites the alpha channel)
