package ui

import "testing"

// TestShapedLineHeight spaces the lines of shaped text as Font.LineHeight
// says, and the caret follows them, as Element.LineHeight does for Text
// (issue #807).
func TestShapedLineHeight(t *testing.T) {
	plain := Font{Size: 20}
	tall := Font{Size: 20, LineHeight: 2}
	_, h := ShapeRichText([]Span{{Text: "a\nb"}}, tall, 0).Size()
	if h < 79 || h > 81 {
		t.Errorf("two lines at 2x of 20 DIPs are %v tall, want 80", h)
	}
	if _, ph := ShapeText("a\nb", plain, 0).Size(); ph >= h {
		t.Errorf("the font's own lines (%v) are not shorter than 2x (%v)", ph, h)
	}
	l := ShapeText("a\nb", tall, 0)
	if y := l.Caret(2).Y - l.Caret(0).Y; y < 39 || y > 41 {
		t.Errorf("the second line's caret is %v below the first, want 40", y)
	}
	if plain.Metrics() != tall.Metrics() {
		t.Errorf("Metrics follow LineHeight: %+v, want the font's own %+v", tall.Metrics(), plain.Metrics())
	}
}
