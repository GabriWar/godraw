package main

// Faithful Go port of steveruizok/perfect-freehand (getStroke pipeline).
// Source: getStrokePoints.ts + getStrokeOutlinePoints.ts + getStrokeRadius.ts
//         + simulatePressure.ts + vec.ts + constants.ts
// The scratch-buffer micro-optimizations of the upstream are dropped in favor
// of plain value-returning vector helpers; the math is 1:1.

import "math"

// Vec2 is a 2D vector [x, y].
type Vec2 = [2]float64

const (
	rateOfPressureChange = 0.275
	fixedPI              = math.Pi + 0.0001
	startCapSegments     = 13
	endCapSegments       = 29
	cornerCapSegments    = 13
	endNoiseThreshold    = 3.0
	minStreamlineT       = 0.15
	streamlineTRange     = 0.85
	minRadius            = 0.01
	defaultFirstPressure = 0.25
	defaultPressure      = 0.5
)

var unitOffset = Vec2{1, 1}

// StrokeOptions mirrors perfect-freehand's StrokeOptions. Taper is expressed as
// a distance in points (0 = no taper); caps default on.
type StrokeOptions struct {
	Size             float64
	Thinning         float64
	Smoothing        float64
	Streamline       float64
	Easing           func(float64) float64
	SimulatePressure bool
	Last             bool
	CapStart         bool
	CapEnd           bool
	TaperStart       float64
	TaperEnd         float64
	TaperStartEase   func(float64) float64
	TaperEndEase     func(float64) float64
}

// StrokePoint is an adjusted point with metadata, as returned by getStrokePoints.
type StrokePoint struct {
	Point         Vec2
	Pressure      float64
	Distance      float64
	Vector        Vec2
	RunningLength float64
}

// ---- vector helpers (vec.ts) ----

func neg(a Vec2) Vec2        { return Vec2{-a[0], -a[1]} }
func add(a, b Vec2) Vec2     { return Vec2{a[0] + b[0], a[1] + b[1]} }
func sub(a, b Vec2) Vec2     { return Vec2{a[0] - b[0], a[1] - b[1]} }
func mul(a Vec2, n float64) Vec2 { return Vec2{a[0] * n, a[1] * n} }
func per(a Vec2) Vec2        { return Vec2{a[1], -a[0]} }
func dpr(a, b Vec2) float64  { return a[0]*b[0] + a[1]*b[1] }
func isEqual(a, b Vec2) bool { return a[0] == b[0] && a[1] == b[1] }
func vlen(a Vec2) float64    { return math.Hypot(a[0], a[1]) }
func dist(a, b Vec2) float64 { return math.Hypot(a[1]-b[1], a[0]-b[0]) }

func dist2(a, b Vec2) float64 {
	dx, dy := a[0]-b[0], a[1]-b[1]
	return dx*dx + dy*dy
}

func uni(a Vec2) Vec2 {
	l := vlen(a)
	if l == 0 {
		return Vec2{0, 0}
	}
	return Vec2{a[0] / l, a[1] / l}
}

func rotAround(a, c Vec2, r float64) Vec2 {
	s, co := math.Sin(r), math.Cos(r)
	px, py := a[0]-c[0], a[1]-c[1]
	return Vec2{px*co - py*s + c[0], px*s + py*co + c[1]}
}

func lrp(a, b Vec2, t float64) Vec2 { return add(a, mul(sub(b, a), t)) }
func prj(a, b Vec2, c float64) Vec2 { return add(a, mul(b, c)) }

func lrp3(a, b [3]float64, t float64) [3]float64 {
	// Interpolated points carry no pressure -> -1 sentinel (use default downstream).
	return [3]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, -1}
}

func pickPressure(p, def float64) float64 {
	if p >= 0 {
		return p
	}
	return def
}

// ---- pressure simulation (simulatePressure.ts) ----

func simulatePressure(prev, distance, size float64) float64 {
	sp := math.Min(1, distance/size)
	rp := math.Min(1, 1-sp)
	return math.Min(1, prev+(rp-prev)*(sp*rateOfPressureChange))
}

// ---- radius (getStrokeRadius.ts) ----

func getStrokeRadius(size, thinning, pressure float64, easing func(float64) float64) float64 {
	return size * easing(0.5-thinning*(0.5-pressure))
}

// ---- getStrokePoints ----

func getStrokePoints(points [][3]float64, o StrokeOptions) []StrokePoint {
	streamline := o.Streamline
	size := o.Size
	isComplete := o.Last

	if len(points) == 0 {
		return nil
	}

	t := minStreamlineT + (1-streamline)*streamlineTRange

	// Copy so we never mutate the input.
	pts := make([][3]float64, len(points))
	copy(pts, points)

	// Two points: add extra interpolated points to avoid "dash" tapers.
	if len(pts) == 2 {
		first, last := pts[0], pts[1]
		pts = pts[:1]
		for i := 1; i < 5; i++ {
			pts = append(pts, lrp3(first, last, float64(i)/4))
		}
	}
	// One point: add a second point a unit away.
	if len(pts) == 1 {
		p0 := pts[0]
		pts = append(pts, [3]float64{p0[0] + unitOffset[0], p0[1] + unitOffset[1], p0[2]})
	}

	strokePoints := []StrokePoint{{
		Point:         Vec2{pts[0][0], pts[0][1]},
		Pressure:      pickPressure(pts[0][2], defaultFirstPressure),
		Vector:        unitOffset,
		Distance:      0,
		RunningLength: 0,
	}}

	hasReachedMinimumLength := false
	runningLength := 0.0
	prev := strokePoints[0]
	max := len(pts) - 1

	for i := 1; i < len(pts); i++ {
		var point Vec2
		if isComplete && i == max {
			point = Vec2{pts[i][0], pts[i][1]}
		} else {
			point = lrp(prev.Point, Vec2{pts[i][0], pts[i][1]}, t)
		}

		if isEqual(prev.Point, point) {
			continue
		}

		distance := dist(point, prev.Point)
		runningLength += distance

		if i < max && !hasReachedMinimumLength {
			if runningLength < size {
				continue
			}
			hasReachedMinimumLength = true
		}

		prev = StrokePoint{
			Point:         point,
			Pressure:      pickPressure(pts[i][2], defaultPressure),
			Vector:        uni(sub(prev.Point, point)),
			Distance:      distance,
			RunningLength: runningLength,
		}
		strokePoints = append(strokePoints, prev)
	}

	if len(strokePoints) >= 2 {
		strokePoints[0].Vector = strokePoints[1].Vector
	} else {
		strokePoints[0].Vector = Vec2{0, 0}
	}
	return strokePoints
}

// ---- cap / dot helpers (getStrokeOutlinePoints.ts) ----

func drawDot(center Vec2, radius float64) []Vec2 {
	offsetPoint := add(center, Vec2{1, 1})
	start := prj(center, uni(per(sub(center, offsetPoint))), -radius)
	out := []Vec2{}
	step := 1.0 / startCapSegments
	for t := step; t <= 1; t += step {
		out = append(out, rotAround(start, center, fixedPI*2*t))
	}
	return out
}

func drawRoundStartCap(center, rightPoint Vec2, segments int) []Vec2 {
	out := []Vec2{}
	step := 1.0 / float64(segments)
	for t := step; t <= 1; t += step {
		out = append(out, rotAround(rightPoint, center, fixedPI*t))
	}
	return out
}

func drawFlatStartCap(center, leftPoint, rightPoint Vec2) []Vec2 {
	cv := sub(leftPoint, rightPoint)
	a := mul(cv, 0.5)
	b := mul(cv, 0.51)
	return []Vec2{sub(center, a), sub(center, b), add(center, b), add(center, a)}
}

func drawRoundEndCap(center, direction Vec2, radius float64, segments int) []Vec2 {
	out := []Vec2{}
	start := prj(center, direction, radius)
	step := 1.0 / float64(segments)
	for t := step; t < 1; t += step {
		out = append(out, rotAround(start, center, fixedPI*3*t))
	}
	return out
}

func drawFlatEndCap(center, direction Vec2, radius float64) []Vec2 {
	return []Vec2{
		add(center, mul(direction, radius)),
		add(center, mul(direction, radius*0.99)),
		sub(center, mul(direction, radius*0.99)),
		sub(center, mul(direction, radius)),
	}
}

func computeInitialPressure(points []StrokePoint, simulate bool, size float64) float64 {
	acc := points[0].Pressure
	n := len(points)
	if n > 10 {
		n = 10
	}
	for i := 0; i < n; i++ {
		pressure := points[i].Pressure
		if simulate {
			pressure = simulatePressure(acc, points[i].Distance, size)
		}
		acc = (acc + pressure) / 2
	}
	return acc
}

// ---- getStrokeOutlinePoints ----

func getStrokeOutlinePoints(points []StrokePoint, o StrokeOptions) []Vec2 {
	size := o.Size
	smoothing := o.Smoothing
	thinning := o.Thinning
	simulate := o.SimulatePressure
	isComplete := o.Last

	easing := o.Easing
	if easing == nil {
		easing = func(t float64) float64 { return t }
	}
	capStart, capEnd := o.CapStart, o.CapEnd
	taperStartEase := o.TaperStartEase
	if taperStartEase == nil {
		taperStartEase = func(t float64) float64 { return t * (2 - t) }
	}
	taperEndEase := o.TaperEndEase
	if taperEndEase == nil {
		taperEndEase = func(t float64) float64 { t = t - 1; return t*t*t + 1 }
	}

	if len(points) == 0 || size <= 0 {
		return nil
	}

	totalLength := points[len(points)-1].RunningLength
	taperStart := o.TaperStart
	taperEnd := o.TaperEnd
	minDistance := (size * smoothing) * (size * smoothing)

	leftPts := []Vec2{}
	rightPts := []Vec2{}

	prevPressure := computeInitialPressure(points, simulate, size)
	radius := getStrokeRadius(size, thinning, points[len(points)-1].Pressure, easing)
	firstRadius := -1.0
	prevVector := points[0].Vector
	prevLeftPoint := points[0].Point
	prevRightPoint := prevLeftPoint
	tempLeftPoint := prevLeftPoint
	tempRightPoint := prevRightPoint
	isPrevPointSharpCorner := false

	for i := 0; i < len(points); i++ {
		pressure := points[i].Pressure
		point := points[i].Point
		vector := points[i].Vector
		distance := points[i].Distance
		runningLength := points[i].RunningLength
		isLastPoint := i == len(points)-1

		// Remove noise from the end of the line.
		if !isLastPoint && totalLength-runningLength < endNoiseThreshold {
			continue
		}

		// Radius from pressure (real or simulated), or half size if no thinning.
		if thinning != 0 {
			if simulate {
				pressure = simulatePressure(prevPressure, distance, size)
			}
			radius = getStrokeRadius(size, thinning, pressure, easing)
		} else {
			radius = size / 2
		}

		if firstRadius < 0 {
			firstRadius = radius
		}

		// Tapering.
		taperStartStrength := 1.0
		if taperStart > 0 && runningLength < taperStart {
			taperStartStrength = taperStartEase(runningLength / taperStart)
		}
		taperEndStrength := 1.0
		if taperEnd > 0 && totalLength-runningLength < taperEnd {
			taperEndStrength = taperEndEase((totalLength - runningLength) / taperEnd)
		}
		radius = math.Max(minRadius, radius*math.Min(taperStartStrength, taperEndStrength))

		// Sharp-corner detection via dot products.
		var nextVector Vec2
		if !isLastPoint {
			nextVector = points[i+1].Vector
		} else {
			nextVector = points[i].Vector
		}
		nextDpr := 1.0
		if !isLastPoint {
			nextDpr = dpr(vector, nextVector)
		}
		prevDpr := dpr(vector, prevVector)

		isPointSharpCorner := prevDpr < 0 && !isPrevPointSharpCorner
		isNextPointSharpCorner := nextDpr < 0

		if isPointSharpCorner || isNextPointSharpCorner {
			offset := mul(per(prevVector), radius)
			step := 1.0 / cornerCapSegments
			for t := 0.0; t <= 1; t += step {
				tl := rotAround(sub(point, offset), point, fixedPI*t)
				leftPts = append(leftPts, tl)
				tempLeftPoint = tl

				tr := rotAround(add(point, offset), point, fixedPI*-t)
				rightPts = append(rightPts, tr)
				tempRightPoint = tr
			}
			prevLeftPoint = tempLeftPoint
			prevRightPoint = tempRightPoint
			if isNextPointSharpCorner {
				isPrevPointSharpCorner = true
			}
			continue
		}
		isPrevPointSharpCorner = false

		if isLastPoint {
			offset := mul(per(vector), radius)
			leftPts = append(leftPts, sub(point, offset))
			rightPts = append(rightPts, add(point, offset))
			continue
		}

		// Regular points: project to either side, smoothing across the corner.
		offset := mul(per(lrp(nextVector, vector, nextDpr)), radius)

		tl := sub(point, offset)
		if i <= 1 || dist2(prevLeftPoint, tl) > minDistance {
			leftPts = append(leftPts, tl)
			prevLeftPoint = tl
		}

		tr := add(point, offset)
		if i <= 1 || dist2(prevRightPoint, tr) > minDistance {
			rightPts = append(rightPts, tr)
			prevRightPoint = tr
		}

		prevPressure = pressure
		prevVector = vector
	}

	// Caps.
	firstPoint := points[0].Point
	var lastPoint Vec2
	if len(points) > 1 {
		lastPoint = points[len(points)-1].Point
	} else {
		lastPoint = add(points[0].Point, Vec2{1, 1})
	}

	startCap := []Vec2{}
	endCap := []Vec2{}

	if len(points) == 1 {
		if !(taperStart > 0 || taperEnd > 0) || isComplete {
			r := firstRadius
			if r < 0 {
				r = radius
			}
			return drawDot(firstPoint, r)
		}
	} else {
		switch {
		case taperStart > 0:
			// tapered start, no cap
		case capStart && len(rightPts) > 0:
			startCap = append(startCap, drawRoundStartCap(firstPoint, rightPts[0], startCapSegments)...)
		case len(leftPts) > 0 && len(rightPts) > 0:
			startCap = append(startCap, drawFlatStartCap(firstPoint, leftPts[0], rightPts[0])...)
		}

		direction := per(neg(points[len(points)-1].Vector))
		switch {
		case taperEnd > 0:
			endCap = append(endCap, lastPoint)
		case capEnd:
			endCap = append(endCap, drawRoundEndCap(lastPoint, direction, radius, endCapSegments)...)
		default:
			endCap = append(endCap, drawFlatEndCap(lastPoint, direction, radius)...)
		}
	}

	// Winding order: left side, end cap, right side reversed, start cap.
	rev := make([]Vec2, len(rightPts))
	for i, p := range rightPts {
		rev[len(rightPts)-1-i] = p
	}
	out := make([]Vec2, 0, len(leftPts)+len(endCap)+len(rev)+len(startCap))
	out = append(out, leftPts...)
	out = append(out, endCap...)
	out = append(out, rev...)
	out = append(out, startCap...)
	return out
}

// getStroke returns the filled outline polygon for a set of input points.
func getStroke(points [][3]float64, o StrokeOptions) []Vec2 {
	return getStrokeOutlinePoints(getStrokePoints(points, o), o)
}
