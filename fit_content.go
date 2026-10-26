package mygo

import (
	"math"

	"github.com/egoist/mygo/internal/platform"
)

// fitContent is what a window that takes the size of its page's content
// (FlyoutOptions.FitContent, WindowOptions.FitContent) keeps: the limits
// of that size, the zoom its page's scripts were made at, the size it
// last took, and the content's last size in CSS pixels, which another
// zoom makes another size. Main thread only.
type fitContent struct {
	min, max  platform.Size // DIPs, 0 for none
	zoom      float64
	size      platform.Size
	cssWidth  float64
	cssHeight float64
	told      bool // the page told cssWidth and cssHeight
}

// maxFitDIPs bounds the sizes a page tells, so that no page makes an int
// overflow: no display is that large.
const maxFitDIPs = 1 << 16

// fit returns the size in DIPs, within the limits, of content of width and
// height CSS pixels at zoom, and false for a size no layout gives
// (negative, NaN). Fractions of a pixel take one, so that the content
// fits, but for the error of rounding a layout's fractional sizes.
func (f *fitContent) fit(width, height, zoom float64) (platform.Size, bool) {
	if !(width >= 0 && height >= 0) {
		return platform.Size{}, false
	}
	if zoom <= 0 {
		zoom = 1
	}
	dips := func(css float64, lo, hi int) int {
		v := int(math.Ceil(min(css*zoom, maxFitDIPs) - 1.0/128))
		v = max(v, lo, 1)
		if hi > 0 {
			v = min(v, hi)
		}
		return v
	}
	return platform.Size{
		Width:  dips(width, f.min.Width, f.max.Width),
		Height: dips(height, f.min.Height, f.max.Height),
	}, true
}

// fitContentSize sizes a window that takes its page's size to the size of
// the content, in CSS pixels, as its bridge told it, unless that changes
// nothing. A window, as opposed to a flyout, which the platform keeps in
// the work area, keeps in the work area of its display, and changes only
// in the dimension its content changed in: a size the user gave it holds
// until its content changes again. It does not change while maximized or
// in full screen. Main thread only.
func (w *Window) fitContentSize(width, height float64) {
	f, n := w.fit, w.native
	if f == nil || n == nil {
		return
	}
	s, ok := f.fit(width, height, n.Zoom())
	if !ok {
		return
	}
	f.cssWidth, f.cssHeight, f.told = width, height, true
	if s == f.size {
		return
	}
	if w.flyout != nil {
		f.size = s
		if s != w.flyoutSize {
			w.setFlyoutSize(s.Width, s.Height)
		}
		return
	}
	if n.IsMaximized() || n.IsFullScreen() {
		return
	}
	bounds, content := n.Bounds(), n.ContentBounds()
	if wa := platform.WorkAreaFor(bounds, backend().Screen().Displays()); wa.Width > 0 && wa.Height > 0 {
		s.Width = min(s.Width, max(wa.Width-(bounds.Width-content.Width), 1))
		s.Height = min(s.Height, max(wa.Height-(bounds.Height-content.Height), 1))
	}
	b := content
	if s.Width != f.size.Width {
		b.Width = s.Width
	}
	if s.Height != f.size.Height {
		b.Height = s.Height
	}
	f.size = s
	if b != content {
		n.SetContentBounds(b)
	}
}
