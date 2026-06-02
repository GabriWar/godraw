package main

import (
	"math"

	"gioui.org/f32"
)

// Camera maps world coordinates to screen pixels: screen = world*Zoom + Offset.
type Camera struct {
	Offset f32.Point
	Zoom   float64
}

func (c Camera) ScreenToWorld(p f32.Point) Vec2 {
	return Vec2{
		float64(p.X-c.Offset.X) / c.Zoom,
		float64(p.Y-c.Offset.Y) / c.Zoom,
	}
}

func (c Camera) WorldToScreen(w Vec2) f32.Point {
	return f32.Point{
		X: float32(w[0]*c.Zoom) + c.Offset.X,
		Y: float32(w[1]*c.Zoom) + c.Offset.Y,
	}
}

// ZoomAt scales the zoom by factor while keeping the world point under `screen`
// fixed (zoom-to-cursor).
func (c *Camera) ZoomAt(screen f32.Point, factor float64) {
	before := c.ScreenToWorld(screen)
	c.Zoom = math.Max(0.05, math.Min(40, c.Zoom*factor))
	c.Offset = f32.Point{
		X: screen.X - float32(before[0]*c.Zoom),
		Y: screen.Y - float32(before[1]*c.Zoom),
	}
}
