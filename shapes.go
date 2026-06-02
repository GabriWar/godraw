package main

import "math"

// Shape tools. toolPen is the freehand pen; the rest are geometric shapes whose
// perimeter is fed through the perfect-freehand pipeline so they share the
// hand-drawn outline look of the pen.
const (
	toolPen = iota
	toolRect
	toolEllipse
	toolLine
	toolArrow
)

// shapeStrokes builds the committed outline stroke(s) for the active shape tool
// dragged from p0 to p1 (world coords). Returns nil for degenerate drags.
func (ap *App) shapeStrokes(p0, p1 Vec2) []Stroke {
	if dist(p0, p1) < 2 {
		return nil
	}
	var polylines [][][3]float64
	closed := false
	switch ap.tool {
	case toolRect:
		closed = true
		tl := Vec2{math.Min(p0[0], p1[0]), math.Min(p0[1], p1[1])}
		br := Vec2{math.Max(p0[0], p1[0]), math.Max(p0[1], p1[1])}
		tr := Vec2{br[0], tl[1]}
		bl := Vec2{tl[0], br[1]}
		polylines = append(polylines, densify(tl, tr, br, bl, tl))
	case toolEllipse:
		closed = true
		cx, cy := (p0[0]+p1[0])/2, (p0[1]+p1[1])/2
		rx, ry := math.Abs(p1[0]-p0[0])/2, math.Abs(p1[1]-p0[1])/2
		const N = 64
		pl := make([][3]float64, 0, N+1)
		for i := 0; i <= N; i++ {
			t := float64(i) / N * 2 * math.Pi
			pl = append(pl, [3]float64{cx + rx*math.Cos(t), cy + ry*math.Sin(t), 0.5})
		}
		polylines = append(polylines, pl)
	case toolLine:
		polylines = append(polylines, densify(p0, p1))
	case toolArrow:
		polylines = append(polylines, arrowPolylines(p0, p1)...)
	}
	opts := ap.shapeOpts(closed)
	var out []Stroke
	for _, pl := range polylines {
		o := getStroke(pl, opts)
		if len(o) >= 3 {
			out = append(out, Stroke{Outline: o, Color: ap.color})
		}
	}
	return out
}

// ptsOf turns world vectors into a [x,y,pressure] polyline at uniform pressure.
func ptsOf(vs ...Vec2) [][3]float64 {
	pl := make([][3]float64, len(vs))
	for i, v := range vs {
		pl[i] = [3]float64{v[0], v[1], 0.5}
	}
	return pl
}

// arrowPolylines returns shaft + 2 barbs as separate polylines so
// perfect-freehand doesn't mangle the doubled-back tip.
// densify inserts points every ~6 world-units along each segment so
// perfect-freehand can't bow straight edges.
func densify(vs ...Vec2) [][3]float64 {
	const step = 6.0
	var out [][3]float64
	for i := 1; i < len(vs); i++ {
		a, b := vs[i-1], vs[i]
		d := dist(a, b)
		n := int(math.Ceil(d / step))
		if n < 1 {
			n = 1
		}
		for j := 0; j < n; j++ {
			t := float64(j) / float64(n)
			out = append(out, [3]float64{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1]), 0.5})
		}
	}
	last := vs[len(vs)-1]
	out = append(out, [3]float64{last[0], last[1], 0.5})
	return out
}

func arrowPolylines(p0, p1 Vec2) [][][3]float64 {
	dir := uni(sub(p0, p1))
	if isEqual(dir, Vec2{0, 0}) {
		return [][][3]float64{ptsOf(p0, p1)}
	}
	headLen := math.Min(dist(p0, p1)*0.3, 28)
	const ang = 25 * math.Pi / 180
	rot := func(v Vec2, t float64) Vec2 {
		c, s := math.Cos(t), math.Sin(t)
		return Vec2{v[0]*c - v[1]*s, v[0]*s + v[1]*c}
	}
	hl := add(p1, mul(rot(dir, ang), headLen))
	hr := add(p1, mul(rot(dir, -ang), headLen))
	return [][][3]float64{densify(p0, p1), densify(p1, hl), densify(p1, hr)}
}

// shapeOpts gives shapes a uniform-width outline (no thinning, no pressure sim).
// Closed shapes drop the end caps so the outline rings cleanly.
func (ap *App) shapeOpts(closed bool) StrokeOptions {
	return StrokeOptions{
		Size:             ap.size,
		Thinning:         0,
		Smoothing:        0.05,
		Streamline:       0.05,
		Easing:           func(t float64) float64 { return t },
		SimulatePressure: false,
		Last:             true,
		CapStart:         !closed,
		CapEnd:           !closed,
	}
}
