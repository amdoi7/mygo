package editor

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestGutterActionsHaveIndependentHitTargets(t *testing.T) {
	st, tt, fired := gutterFixture(t, "target:\n  body()\nnext:\n  body()", []GutterAction{{Line: 0, Key: "first", Label: "Run target"}, {Line: 2, Key: "next", Label: "Run next"}})
	var lines []int
	st.OnGutterClick = func(line int) { lines = append(lines, line) }
	st.GutterMarks = []GutterMark{{Line: 0, Color: ui.RGB(30, 100, 200)}}
	tt.Frame()
	r := gutterButton(t, tt, "Run target")
	before := st.Selection()
	if err := tt.Click("Run target"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*fired, []string{"first"}) || len(lines) != 0 || st.Selection() != before || len(st.Folded()) != 0 {
		t.Fatalf("action leaked to editor: fired=%v lines=%v selection=%+v", *fired, lines, st.Selection())
	}
	tt.ClickAt(r.X+r.W+6, r.Y+r.H/2)
	if !slices.Equal(lines, []int{0}) || len(*fired) != 1 {
		t.Fatalf("marker leaked to action: fired=%v lines=%v", *fired, lines)
	}
	x := st.ed.foldX() + 2
	tt.Move(x, r.Y+r.H/2)
	tt.ClickAt(x, r.Y+r.H/2)
	if !slices.Equal(st.Folded(), []int{0}) || len(*fired) != 1 {
		t.Fatalf("fold leaked to action: folds=%v fired=%v", st.Folded(), *fired)
	}
}

func TestGutterActionsFollowCurrentWrapScrollAndResize(t *testing.T) {
	st, tt, fired := gutterFixture(t, strings.Repeat("wrap word ", 20)+"\ntarget\n"+strings.Repeat("line\n", 80), []GutterAction{{Line: 1, Key: "target", Label: "Run target"}, {Line: 60, Key: "far", Label: "Run far"}})
	check := func(label string, line int) {
		t.Helper()
		r, ok := tt.Find(label)
		y := float32(st.ed.rowTop(line) - st.ed.scrollY)
		visible := y+st.ed.view.lineHeight > 0 && y < st.ed.viewH
		if visible && (!ok || math.Abs(float64(r.Y-max(0, y))) > 0.1 || math.Abs(float64(r.H-(min(st.ed.viewH, y+st.ed.view.lineHeight)-max(0, y)))) > 0.1) {
			t.Fatalf("stale action geometry: %s bounds=%+v found=%v want_y=%v", label, r, ok, y)
		}
		if !visible && ok {
			t.Fatalf("offscreen action exposed: %s %+v", label, r)
		}
	}
	check("Run target", 1)
	tt.SetSize(260, 300)
	check("Run target", 1)
	st.Font.Size = 18
	tt.Frame()
	check("Run target", 1)
	st.ScrollTo(60)
	tt.Frame()
	check("Run far", 60)
	check("Run target", 1)
	tt.Scroll(150, 120, 0, 7)
	check("Run far", 60)
	if err := tt.Click("Run far"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*fired, []string{"far"}) {
		t.Fatalf("scrolled action fired %v", *fired)
	}
	tt.SetScale(2)
	check("Run far", 60)
}

func TestGutterActionsHideFoldedAndInvalidLines(t *testing.T) {
	st, tt, fired := gutterFixture(t, "if x {\n  target()\n}\n", []GutterAction{{Line: 0, Key: "outer", Label: "Run outer"}, {Line: 1, Key: "inner", Label: "Run inner"}, {Line: -1, Key: "negative", Label: "Invalid negative"}, {Line: 100, Key: "past", Label: "Invalid past"}})
	st.Fold(0)
	tt.Frame()
	for _, label := range []string{"Run inner", "Invalid negative", "Invalid past"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("hidden action exposed: %s", label)
		}
	}
	if err := tt.Click("Run outer"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*fired, []string{"outer"}) || !slices.Equal(st.Folded(), []int{0}) {
		t.Fatalf("Run changed fold: fired=%v folded=%v", *fired, st.Folded())
	}
	st.Unfold(0)
	tt.Frame()
	if err := tt.Click("Run inner"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*fired, []string{"outer", "inner"}) {
		t.Fatalf("unfolded target=%v", *fired)
	}
}

func TestGutterActionKeyboardActivation(t *testing.T) {
	_, tt, fired := gutterFixture(t, "target\n", []GutterAction{{Line: 0, Key: "target", Label: "Run target"}})
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Run target") {
		t.Fatal("gutter action cannot receive keyboard focus")
	}
	tt.Key(0, ui.KeyEnter)
	if !slices.Equal(*fired, []string{"target"}) {
		t.Fatalf("keyboard action=%v", *fired)
	}
}

func TestGutterActionsRevealUsesResizedWidth(t *testing.T) {
	long := strings.Repeat("wrap word ", 20)
	st, tt, _ := gutterFixture(t, long+"\ntarget\n"+strings.Repeat("line\n", 40), []GutterAction{{Line: 1, Key: "target", Label: "Run target"}})
	st.Select(len(long)+1, len(long)+1)
	tt.SetSize(200, 120)
	r, ok := tt.Find("Run target")
	if !ok || r.Y < 0 || r.Y+r.H > 120 {
		t.Fatalf("selected target not revealed after resize: %+v found=%v scroll=%v", r, ok, st.ed.scrollY)
	}
}

func gutterFixture(t *testing.T, source string, actions []GutterAction) (*State, *ui.Tester, *[]string) {
	t.Helper()
	st := &State{Buffer: NewBuffer(source), ReadOnly: true, SoftWrap: true, NoScrollPastEnd: true, Font: ui.Font{Size: 14}, GutterActions: actions}
	var fired []string
	st.RenderGutterAction = func(c *ui.Context, button ui.Element, action GutterAction) {
		button.Center().Children(func() { ui.Text(c, "run").FontSize(10) })
		if button.Clicked() {
			fired = append(fired, action.Key)
		}
	}
	tt := ui.NewTester(func(c *ui.Context) { View(c, st).Fill().AutoFocus() }, 400, 300)
	return st, tt, &fired
}

func gutterButton(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing action %s", label)
	}
	return r
}
