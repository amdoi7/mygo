package ui

type surfaceDock struct {
	slot, surface *node
	memory        *surfaceFootprint
	detached      bool
}

type surfaceFootprint struct {
	height   float32
	detached bool
}

// DockedSurface builds one overlay column whose in-flow slot measures its
// content at the slot's width. Detached surfaces retain the last docked
// footprint (zero before docking) and use their own absolute placement.
// The slot must be an empty in-flow box outside the surface's ancestry.
func DockedSurface(c *Context, slot Element, detached bool) Element {
	anchor := slot.node()
	raw := c.build()
	if raw == nil || anchor == nil || anchor.c != raw || anchor.kind != kindBox || anchor.first != nil || anchor.dock != nil || anchor.flags&flagAbsolute != 0 || anchor.height.u != unitAuto {
		panic("ui: DockedSurface requires an empty in-flow slot in the current context without an explicit height")
	}
	for parent := raw.parent; parent != nil; parent = parent.parent {
		if parent == anchor {
			panic("ui: DockedSurface cannot be built inside its slot")
		}
	}
	surface := Column(c).Absolute()
	memory := coreLocal(anchor, "surface-footprint", func() surfaceFootprint { return surfaceFootprint{} })
	dock := &surfaceDock{slot: anchor, surface: surface.node(), memory: memory, detached: detached}
	anchor.dock, dock.surface.dock = dock, dock
	raw.rt.keepPart(dock)
	return surface
}

// Docked reports whether the last presented surface reached its slot and the
// current build does not detach it. A return keeps requesting frames until
// that presented geometry is settled, including with reduced motion.
func (e Element) Docked() bool {
	n := e.node()
	if n == nil || n.dock == nil || n.dock.surface != n || n.dock.detached {
		return false
	}
	moving := n.dock.memory.detached
	if transition := n.st.trec; transition != nil && transition.built+1 >= n.c.rt.frame {
		moving = moving || transition.moving&(movePos|moveSize) != 0
	}
	if moving {
		n.c.rt.animating = true
	}
	return !moving
}

func (d *surfaceDock) height(width, parentHeight float32) float32 {
	d.memory.detached = d.detached
	if !d.detached {
		d.memory.height = heightAt(d.surface, width, parentHeight)
	}
	return d.memory.height
}

func (d *surfaceDock) place(parent *node) {
	box := laidOutBox(d.slot)
	x, y := laidOutOrigin(parent)
	d.surface.x, d.surface.y = box.X-x, box.Y-y
	if d.surface.w != box.W || d.surface.h != box.H {
		layoutBox(d.surface, box.W, box.H)
	}
}

func (rt *engine) placeDockedSurfaces() {
	for _, part := range rt.parts {
		dock, ok := part.(*surfaceDock)
		if !ok || dock.detached {
			continue
		}
		if transition := dock.surface.st.trec; transition != nil && transition.laid == rt.frame && transition.moving&(movePos|moveSize) != 0 {
			continue
		}
		dock.place(dock.surface.parent)
	}
}
