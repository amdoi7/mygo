//go:build darwin

package darwin

import (
	"slices"

	"github.com/ebitengine/purego/objc"
)

// A window with a Parent stays above it by being ordered there, not
// attached as an AppKit child window (addChildWindow:ordered:). AppKit
// keeps a child window on its parent's Space, and each display has Spaces
// of its own: moved to another display, a child window is not drawn
// there, though the window server lists it, and the clicks in its frame
// go to the windows behind. Detached with removeChildWindow:, it stays on
// its parent's Space all the same. AppKit also keeps child windows in the
// order they were attached in, whichever the user brings in front. An
// ordinary window goes to the Space of the display it is on, so a child
// window stays one: it is ordered above its parent and the parent's
// other children as it shows, and the parent's children are ordered
// above it again whenever the parent is ordered in
// (orderWindowRelativeTo), takes the keyboard or is restored from the
// Dock, in the order they are in. It hides with its
// parent, as AppKit's child windows do, and shows again with it. Sheets
// and flyouts are still attached, as AppKit attaches them.

// isChild tells that w is a child window kept above its parent: one with
// a Parent that is neither a sheet, nor a flyout or popover, nor a web
// view in native UI.
func (w *window) isChild() bool {
	return w.parent != nil && !w.parent.closed && !w.closed &&
		w.host == nil && w.opts.Flyout == nil && !w.opts.Modal && !w.sheet
}

// onScreen tells that a window shows: visible and not in the Dock.
func (w *window) onScreen() bool {
	return !w.closed && sendBool(w.win, "isVisible") && !sendBool(w.win, "isMiniaturized")
}

// showAboveParent shows w, as the key window for Show (key): a child
// window above its parent and the parent's other child windows, as AppKit
// puts the child window attached last, or as any window while its parent
// does not show. Then its own children go above it.
func (w *window) showAboveParent(key bool) {
	w.hiddenWithParent = false
	switch {
	case w.isChild() && w.parent.onScreen():
		w.b.orderingChildren = true
		w.orderAbove(w.parent.frontmost(w))
		w.b.orderingChildren = false
		if key {
			send(w.win, "makeKeyWindow")
		}
	case key:
		send(w.win, "makeKeyAndOrderFront:", 0)
	default:
		send(w.win, "orderFrontRegardless")
	}
	w.orderChildrenLater()
}

// frontmost returns the frontmost window that shows of w and the child
// windows above it, with theirs, leaving out skip and its own.
func (w *window) frontmost(skip *window) id {
	top := w.win
	withPool(func() {
		for _, x := range arrayItems(send(w.b.app, "orderedWindows")) {
			c := w.b.byNSWindow[x]
			switch {
			case c == w:
				return
			case c != nil && c != skip && c.onScreen() && c.descends(w) && !c.descends(skip):
				top = x
				return
			}
		}
	})
	return top
}

// descends tells that w is a child window kept above a, or above one of
// a's.
func (w *window) descends(a *window) bool {
	for c := w; c.isChild(); c = c.parent {
		if c.parent == a {
			return true
		}
	}
	return false
}

// orderAbove orders w right above the window rel.
func (w *window) orderAbove(rel id) {
	send(w.win, "orderWindow:relativeTo:", 1, uintptr(sendInt(rel, "windowNumber"))) // NSWindowAbove
}

// orderChildrenLater orders w's child windows above it once the current
// event is handled: AppKit is still ordering w, or making it key, when it
// tells. Once for the calls made before then.
func (w *window) orderChildrenLater() {
	if w.orderPending || w.closed {
		return
	}
	w.orderPending = true
	w.b.post(func() {
		w.orderPending = false
		if w.closed || w.b.orderingChildren || !w.onScreen() || !w.hasChildren() {
			return
		}
		w.b.orderingChildren = true
		w.orderChildren()
		w.b.orderingChildren = false
	})
}

// hasChildren tells that w has a child window kept above it.
func (w *window) hasChildren() bool {
	for _, c := range w.b.byNSWindow {
		if c.parent == w && c.isChild() {
			return true
		}
	}
	return false
}

// orderChildren orders w's child windows that show, and those that hid
// with it, right above it, in the order they are in, each with its own
// children above it, and returns the frontmost window it ordered, or w's.
func (w *window) orderChildren() id {
	top := w.win
	kids := w.children(true)
	for i := len(kids) - 1; i >= 0; i-- { // back to front
		c := kids[i]
		c.hiddenWithParent = false
		c.orderAbove(top)
		top = c.orderChildren()
	}
	return top
}

// hideChildren orders out w's child windows that show, and theirs, as w
// hides or miniaturizes, remembering them to show again with it.
func (w *window) hideChildren() {
	if w.b.orderingChildren {
		return
	}
	w.b.orderingChildren = true
	defer func() { w.b.orderingChildren = false }()
	w.hideChildrenNow()
}

func (w *window) hideChildrenNow() {
	for i, c := range w.children(false) {
		c.hiddenWithParent, c.hiddenRank = true, i
		send(c.win, "orderOut:", 0)
		c.hideChildrenNow()
	}
}

// children returns w's child windows that show, front to back, then,
// with hidden, those that hid with it, as they were.
func (w *window) children(hidden bool) []*window {
	var front, gone []*window
	withPool(func() {
		for _, x := range arrayItems(send(w.b.app, "orderedWindows")) {
			if c := w.b.byNSWindow[x]; c != nil {
				front = append(front, c)
			}
		}
	})
	if hidden {
		for _, c := range w.b.byNSWindow {
			if c.parent == w && c.isChild() && c.hiddenWithParent && !c.onScreen() {
				gone = append(gone, c)
			}
		}
	}
	return childOrder(front, func(c *window) bool { return c.parent == w && c.isChild() && c.onScreen() },
		gone, func(c *window) int { return c.hiddenRank })
}

// childOrder returns the child windows to order above their parent, front
// to back: those of the app's windows front to back (front) that shows
// tells show, in that order, then those that hid with their parent
// (hidden), by rank, the frontmost 0.
func childOrder[W comparable](front []W, shows func(W) bool, hidden []W, rank func(W) int) []W {
	var kids []W
	for _, x := range front {
		if shows(x) {
			kids = append(kids, x)
		}
	}
	hidden = slices.Clone(hidden)
	slices.SortStableFunc(hidden, func(a, b W) int { return rank(a) - rank(b) })
	for _, x := range hidden {
		if !slices.Contains(kids, x) {
			kids = append(kids, x)
		}
	}
	return kids
}

// orderWindowRelativeTo overrides orderWindow:relativeTo: of the class
// named, which AppKit orders a window in or out with whatever orders it:
// orderFront:, makeKeyAndOrderFront:, orderFrontRegardless, orderBack:,
// a click, the Dock, Mission Control, assistive technology raising it. A
// window ordered in orders its child windows above it again once AppKit
// is done, for the key window too, which tells nothing else as it comes
// in front of its children. The orderings of the children themselves,
// while orderingChildren, order nothing more.
func orderWindowRelativeTo(className string) func(self id, cmd objc.SEL, place, other int) {
	return func(self id, cmd objc.SEL, place, other int) {
		sendSuper(self, className, cmd, uintptr(place), uintptr(other))
		const out = 0 // NSWindowOut
		if b := theBackend; b != nil && place != out && !b.orderingChildren {
			if w := b.byNSWindow[self]; w != nil {
				w.orderChildrenLater()
			}
		}
	}
}
