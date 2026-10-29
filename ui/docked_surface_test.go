package ui

import (
	"testing"
	"time"
)

func TestDockedSurfaceMeasuresAndPlacesInTheSameFrame(t *testing.T) {
	height := float32(40)
	clicks := 0
	tt := NewTester(func(c *Context) {
		var slot Element
		Column(c).Fill().Padding(10, 20).Children(func() {
			Box(c).Grow(1)
			slot = Box(c.Key("slot")).Label("slot").FillWidth()
		})
		Overlay(c, func() {
			Column(c).Fill().PassThrough().Children(func() {
				DockedSurface(c.Key("surface"), slot, false).Label("surface").Children(func() {
					if ButtonBase(c).Label("action").FillWidth().Height(height).Clicked() {
						clicks++
					}
				})
			})
		})
	}, 400, 300)
	for _, size := range []float32{40, 72, 110, 53} {
		height = size
		tt.Frame()
		slot, _ := tt.Find("slot")
		surface, _ := tt.Find("surface")
		if slot != surface || surface.H != height || surface.Y+surface.H != 290 {
			t.Fatalf("layout split: slot=%+v surface=%+v height=%g", slot, surface, height)
		}
		if err := tt.Click("action"); err != nil {
			t.Fatal(err)
		}
	}
	if clicks != 4 {
		t.Fatalf("displayed action lost its hit region: clicks=%d", clicks)
	}
}

func TestDockedSurfaceRetainsItsFootprintUntilItReturns(t *testing.T) {
	detached := false
	docked := false
	tt := NewTester(func(c *Context) {
		var slot Element
		Column(c).Fill().Padding(10, 20).Children(func() {
			Box(c).Grow(1)
			slot = Box(c.Key("slot")).Label("slot").FillWidth()
		})
		Overlay(c, func() {
			Column(c).Fill().PassThrough().Children(func() {
				surface := DockedSurface(c.Key("surface"), slot, detached).Label("surface")
				if detached {
					surface.Left(50).Top(30).Width(200).MinHeight(160)
				}
				surface.Transition(ElementTransition{Duration: 100 * time.Millisecond, Position: true, Size: true}).Children(func() { Box(c).Height(40) })
				docked = surface.Docked()
			})
		})
	}, 400, 300)
	detached = true
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	slot, _ := tt.Find("slot")
	surface, _ := tt.Find("surface")
	if slot.H != 40 || surface.H != 160 || docked {
		t.Fatalf("detached surface changed the flow: slot=%+v surface=%+v docked=%t", slot, surface, docked)
	}
	detached = false
	tt.Frame()
	if docked {
		t.Fatal("return released its barrier before presentation reached the dock")
	}
	tt.Advance(200 * time.Millisecond)
	slot, _ = tt.Find("slot")
	surface, _ = tt.Find("surface")
	if !docked || surface != slot || tt.FramePending() {
		t.Fatalf("return did not settle: slot=%+v surface=%+v docked=%t pending=%t", slot, surface, docked, tt.FramePending())
	}
}

func TestDockedSurfaceFollowsPresentedAncestor(t *testing.T) {
	left := float32(0)
	tt := NewTester(func(c *Context) {
		var slot Element
		Column(c).Fill().Children(func() {
			Column(c.Key("moving-parent")).Absolute().Left(left).Width(300).Height(200).
				Transition(ElementTransition{Duration: 100 * time.Millisecond, Position: true}).Children(func() {
				Box(c).Grow(1)
				slot = Box(c.Key("slot")).Label("slot").FillWidth()
			})
		})
		Overlay(c, func() {
			Column(c).Fill().PassThrough().Children(func() {
				DockedSurface(c.Key("surface"), slot, false).Label("surface").Children(func() { Box(c).Height(40) })
			})
		})
	}, 400, 300)
	left = 50
	tt.Frame()
	for frame := range 20 {
		slot, _ := tt.Find("slot")
		surface, _ := tt.Find("surface")
		if slot != surface {
			t.Fatalf("frame=%d surface followed the target instead of presentation: slot=%+v surface=%+v", frame, slot, surface)
		}
		tt.Advance(8 * time.Millisecond)
	}
}

func TestDockedSurfaceRejectsAnAncestorAsItsSlot(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("cyclic layout dependency was accepted")
		}
	}()
	NewTester(func(c *Context) {
		slot := Column(c)
		slot.Children(func() { DockedSurface(c, slot, false) })
	}, 400, 300)
}
