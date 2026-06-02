package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"os/exec"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

const (
	modeNone = iota
	modeDraw
	modePan
	modeShape
)

var palette = []color.NRGBA{
	{R: 30, G: 30, B: 35, A: 255},   // ink
	{R: 224, G: 49, B: 49, A: 255},  // red
	{R: 28, G: 126, B: 214, A: 255}, // blue
	{R: 47, G: 158, B: 68, A: 255},  // green
}

// Stroke is a committed stroke: its outline polygon in world coordinates.
type Stroke struct {
	Outline []Vec2
	Color   color.NRGBA
}

type App struct {
	th         *material.Theme
	cam        Camera
	strokes    []Stroke
	cur        [][3]float64 // in-progress raw points: [x, y, pressure] (world coords)
	mode       int
	tool       int
	overlay    bool
	lastPan    f32.Point
	shapeStart Vec2 // shape drag anchor (world coords)
	shapeEnd   Vec2 // shape drag current (world coords)
	color      color.NRGBA
	size       float64
}

func newApp(overlay bool) *App {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	return &App{
		th:      th,
		cam:     Camera{Zoom: 1},
		color:   palette[0],
		size:    10,
		overlay: overlay,
	}
}

func monitorSize() (unit.Dp, unit.Dp) {
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err == nil {
		var mons []struct {
			Width   int  `json:"width"`
			Height  int  `json:"height"`
			Focused bool `json:"focused"`
		}
		if json.Unmarshal(out, &mons) == nil {
			for _, m := range mons {
				if m.Focused {
					return unit.Dp(m.Width), unit.Dp(m.Height)
				}
			}
			if len(mons) > 0 {
				return unit.Dp(mons[0].Width), unit.Dp(mons[0].Height)
			}
		}
	}
	return unit.Dp(1920), unit.Dp(1080)
}

func main() {
	overlay := flag.Bool("overlay", false, "transparent overlay mode (gromit-style annotation layer)")
	flag.Parse()
	app.ID = "godraw" // wayland app_id → Hyprland window `class` for windowrules
	if *overlay {
		// Triggers the vendored Gio patch: transparent framebuffer clear + no
		// opaque region, so the desktop shows through (see third_party/gio).
		os.Setenv("GIO_TRANSPARENT", "1")
	}

	go func() {
		w := new(app.Window)
		mw, mh := monitorSize()
		w.Option(
			app.Title("godraw — excalidraw-ish (go/wayland)"),
			app.Size(mw, mh),
		)
		if err := run(w, *overlay); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, overlay bool) error {
	a := newApp(overlay)
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) layout(gtx C) D {
	a.handlePointer(gtx)
	a.handleKeys(gtx)

	// Paper background + grid only in normal mode. Overlay mode leaves the
	// surface transparent so the desktop shows through (gromit-style).
	if !a.overlay {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 250, G: 250, B: 248, A: 255},
			clip.Rect{Max: gtx.Constraints.Max}.Op())
		a.drawGrid(gtx)
	}

	for i := range a.strokes {
		fillPolygon(gtx.Ops, a.cam, a.strokes[i].Outline, a.strokes[i].Color)
	}
	if len(a.cur) > 0 {
		fillPolygon(gtx.Ops, a.cam, getStroke(a.cur, a.strokeOpts(false)), a.color)
	}
	if a.mode == modeShape {
		for _, s := range a.shapeStrokes(a.shapeStart, a.shapeEnd) {
			fillPolygon(gtx.Ops, a.cam, s.Outline, s.Color)
		}
	}

	a.registerInput(gtx)
	a.drawOverlay(gtx)
	return D{Size: gtx.Constraints.Max}
}

func (a *App) registerInput(gtx C) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, a)
	gtx.Execute(key.FocusCmd{Tag: a})
}

func (a *App) handlePointer(gtx C) {
	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target:  a,
			Kinds:   pointer.Press | pointer.Release | pointer.Drag | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -2000, Max: 2000},
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			switch {
			case pe.Buttons&pointer.ButtonSecondary != 0 || pe.Buttons&pointer.ButtonTertiary != 0:
				a.mode = modePan
				a.lastPan = pe.Position
			case pe.Buttons&pointer.ButtonPrimary != 0:
				if a.tool == toolPen {
					a.mode = modeDraw
					a.cur = a.cur[:0]
					a.addPoint(pe.Position)
				} else {
					a.mode = modeShape
					a.shapeStart = a.cam.ScreenToWorld(pe.Position)
					a.shapeEnd = a.shapeStart
				}
			}
		case pointer.Drag:
			switch a.mode {
			case modePan:
				a.cam.Offset = a.cam.Offset.Add(pe.Position.Sub(a.lastPan))
				a.lastPan = pe.Position
			case modeDraw:
				a.addPoint(pe.Position)
			case modeShape:
				a.shapeEnd = a.cam.ScreenToWorld(pe.Position)
			}
		case pointer.Release:
			switch {
			case a.mode == modeDraw && len(a.cur) > 0:
				out := getStroke(a.cur, a.strokeOpts(true))
				a.strokes = append(a.strokes, Stroke{Outline: out, Color: a.color})
			case a.mode == modeShape:
				a.strokes = append(a.strokes, a.shapeStrokes(a.shapeStart, a.shapeEnd)...)
			}
			a.cur = a.cur[:0]
			a.mode = modeNone
		case pointer.Scroll:
			a.cam.ZoomAt(pe.Position, math.Exp(-float64(pe.Scroll.Y)*0.0025))
		}
	}
}

func (a *App) addPoint(p f32.Point) {
	w := a.cam.ScreenToWorld(p)
	a.cur = append(a.cur, [3]float64{w[0], w[1], -1})
}

func (a *App) handleKeys(gtx C) {
	for {
		ev, ok := gtx.Source.Event(
			key.Filter{Name: "C"},
			key.Filter{Name: "Z", Required: key.ModShortcut},
			key.Filter{Name: "S"},
			key.Filter{Name: "0"},
			key.Filter{Name: "1"},
			key.Filter{Name: "2"},
			key.Filter{Name: "3"},
			key.Filter{Name: "4"},
			key.Filter{Name: "P"},
			key.Filter{Name: "R"},
			key.Filter{Name: "O"},
			key.Filter{Name: "L"},
			key.Filter{Name: "A"},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case "C":
			a.strokes = nil
		case "Z":
			if len(a.strokes) > 0 {
				a.strokes = a.strokes[:len(a.strokes)-1]
			}
		case "S":
			a.size = nextSize(a.size)
		case "0":
			a.cam = Camera{Zoom: 1}
		case "1":
			a.color = palette[0]
		case "2":
			a.color = palette[1]
		case "3":
			a.color = palette[2]
		case "4":
			a.color = palette[3]
		case "P":
			a.tool = toolPen
		case "R":
			a.tool = toolRect
		case "O":
			a.tool = toolEllipse
		case "L":
			a.tool = toolLine
		case "A":
			a.tool = toolArrow
		}
	}
}

func toolName(t int) string {
	switch t {
	case toolRect:
		return "rect"
	case toolEllipse:
		return "ellipse"
	case toolLine:
		return "line"
	case toolArrow:
		return "arrow"
	default:
		return "pen"
	}
}

func nextSize(s float64) float64 {
	switch {
	case s < 8:
		return 10
	case s < 14:
		return 18
	default:
		return 6
	}
}

func (a *App) strokeOpts(last bool) StrokeOptions {
	return StrokeOptions{
		Size:             a.size,
		Thinning:         0.6,
		Smoothing:        0.5,
		Streamline:       0.5,
		Easing:           func(t float64) float64 { return math.Sin(t * math.Pi / 2) },
		SimulatePressure: true,
		Last:             last,
		CapStart:         true,
		CapEnd:           true,
	}
}

func fillPolygon(ops *op.Ops, cam Camera, world []Vec2, col color.NRGBA) {
	if len(world) < 3 {
		return
	}
	var path clip.Path
	path.Begin(ops)
	path.MoveTo(cam.WorldToScreen(world[0]))
	for _, wpt := range world[1:] {
		path.LineTo(cam.WorldToScreen(wpt))
	}
	path.Close()
	paint.FillShape(ops, col, clip.Outline{Path: path.End()}.Op())
}

func (a *App) drawGrid(gtx C) {
	const worldStep = 40.0
	step := worldStep * a.cam.Zoom
	if step < 6 {
		return
	}
	w := float64(gtx.Constraints.Max.X)
	h := float64(gtx.Constraints.Max.Y)
	sx := math.Mod(float64(a.cam.Offset.X), step)
	if sx < 0 {
		sx += step
	}
	sy := math.Mod(float64(a.cam.Offset.Y), step)
	if sy < 0 {
		sy += step
	}
	dot := color.NRGBA{R: 213, G: 213, B: 220, A: 255}
	for x := sx; x < w; x += step {
		for y := sy; y < h; y += step {
			ix, iy := int(x), int(y)
			paint.FillShape(gtx.Ops, dot,
				clip.Rect{Min: image.Pt(ix, iy), Max: image.Pt(ix+2, iy+2)}.Op())
		}
	}
}

func (a *App) drawOverlay(gtx C) {
	txt := fmt.Sprintf(
		"tool:%s [P]en [R]ect [O]val [L]ine [A]rrow · 1-4 color · S size(%.0f) · C clear · Ctrl+Z undo · M/R-drag pan · scroll zoom · 0 reset    |    zoom %.0f%%",
		toolName(a.tool), a.size, a.cam.Zoom*100)
	col := color.NRGBA{R: 110, G: 110, B: 120, A: 255}
	if a.overlay {
		col = color.NRGBA{R: 235, G: 235, B: 245, A: 230} // legible against arbitrary desktop
	}
	layout.Inset{Top: unit.Dp(8), Left: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
		l := material.Label(a.th, unit.Sp(12.5), txt)
		l.Color = col
		return l.Layout(gtx)
	})
}
