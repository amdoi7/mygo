package ui

// LayoutPlacement computes an absolute child's rectangle in its parent's current
// border box and controls its visibility. It must not build UI or handle input.
type LayoutPlacement func(parent Rect, scale float32) (rect Rect, visible bool)

// Place positions an absolute child during layout, before painting and hit
// testing. Unlike Bounds during view construction, parent is this frame's box.
func (e Element) Place(placement LayoutPlacement) Element {
	if n := e.node(); n != nil {
		n.flags |= flagAbsolute
		n.placement = placement
	}
	return e
}
