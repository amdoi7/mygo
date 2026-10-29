package editor

import (
	"time"

	"github.com/egoist/mygo/ui"
)

func (ed *editor) hasGutterActions() bool {
	return len(ed.o.gutterActions) > 0 && ed.o.renderGutterAction != nil
}

func (ed *editor) gutterActionWidth() float32 {
	if !ed.hasGutterActions() {
		return 0
	}
	return max(20, ed.view.lineHeight)
}

func (ed *editor) buildGutterActions(c *ui.Context) {
	if !ed.hasGutterActions() {
		return
	}
	var prepared bool
	var box ui.Rect
	var density float32
	prepare := func(parent ui.Rect, scale float32) {
		if prepared && box == parent && density == scale {
			return
		}
		prepared, box, density = true, parent, scale
		ed.r.W, ed.r.H = parent.W, parent.H
		ed.layout(parent.W, parent.H, scale)
		if !ed.view.measureWraps(ed.anchorLine, 4*time.Millisecond) {
			c.AnimationFrame()
		}
		if ed.o.sharedScroll == nil {
			ed.restoreScroll()
			ed.doReveal()
		}
		if ed.autoscroll() {
			c.AnimationFrame()
		}
	}
	for _, action := range ed.o.gutterActions {
		ui.Box(c.Key("gutter action " + action.Key)).Place(func(parent ui.Rect, scale float32) (ui.Rect, bool) {
			prepare(parent, scale)
			if action.Line < 0 || action.Line >= ed.view.text.LineCount() {
				return ui.Rect{}, false
			}
			g := ed.view.heights.geometry(action.Line)
			y := float32(g.rows - ed.scrollY)
			r := ui.Rect{Y: y, W: ed.gutterActionWidth(), H: ed.view.lineHeight}
			return r, !g.hidden && y+r.H > 0 && y < parent.H
		}).Clip().HandleInput(shield).Children(func() {
			button := ui.ButtonBase(c.Key("button")).Label(action.Label).Fill().KeepFocus().Cursor(ui.CursorPointer)
			ed.o.renderGutterAction(c, button, action)
		})
	}
}
