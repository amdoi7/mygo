package ui

import "testing"

func TestPlacementTracksCurrentLayoutAndVisibility(t *testing.T) {
	visible, calls := true, 0
	offset := float32(10)
	var scale float32
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Children(func() {
			button := ButtonBase(c).Label("placed").Place(func(parent Rect, density float32) (Rect, bool) {
				scale = density
				return Rect{X: parent.W - offset - 20, Y: 5, W: 20, H: 20}, visible
			})
			if button.Clicked() {
				calls++
			}
		})
	}, 300, 200)
	if r, ok := tt.Find("placed"); !ok || r.X != 270 {
		t.Fatalf("first layout=%+v found=%v", r, ok)
	}
	offset = 30
	tt.Frame()
	if r, ok := tt.Find("placed"); !ok || r.X != 250 {
		t.Fatalf("dynamic placement was cached: %+v found=%v", r, ok)
	}
	tt.SetSize(400, 200)
	if r, ok := tt.Find("placed"); !ok || r.X != 350 {
		t.Fatalf("current layout=%+v found=%v", r, ok)
	}
	if err := tt.Click("placed"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("clicks=%d", calls)
	}
	tt.SetScale(2)
	if scale != 2 {
		t.Fatalf("scale=%v", scale)
	}
	tt.rt.painted[2] = 0
	layoutTree(tt.rt.c.root, 400, 200)
	if scale != 2 {
		t.Fatalf("placement used a paint cache instead of surface scale: %v", scale)
	}
	visible = false
	tt.Frame()
	tt.ClickAt(360, 15)
	if calls != 1 {
		t.Fatal("hidden placement remained clickable")
	}
}
