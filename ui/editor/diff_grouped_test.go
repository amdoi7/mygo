package editor

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestUnifiedGroupedChangesPaintEveryLineWithCorrectGutters(t *testing.T) {
	old, next := "head\nvalue = 1\nlimit = 2\ntail", "head\nvalue = 3\nlimit = 4\ntail"
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(next), Layout: DiffUnified, ExpandUnchanged: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	ed := &d.eds[1]
	var deletedY float32 = -1
	var blocks int
	ed.eachBlock(0, ed.view.heights.n-1, func(b *block, top float64) {
		if b.kind == blockDeleted {
			blocks++
			if b.from != 1 || b.to != 3 {
				t.Errorf("deleted range=%d:%d", b.from, b.to)
			}
			deletedY = ed.r.Y + float32(top-ed.scrollY)
		}
	})
	if blocks != 1 || deletedY < 0 {
		t.Fatalf("deleted blocks=%d top=%v", blocks, deletedY)
	}
	newFirst := ed.r.Y + float32(ed.view.heights.geometry(1).rows-ed.scrollY)
	newSecond := ed.r.Y + float32(ed.view.heights.geometry(2).rows-ed.scrollY)
	oldSecond := deletedY + ed.view.lineHeight
	if !(deletedY < oldSecond && oldSecond < newFirst && newFirst < newSecond) {
		t.Fatalf("rows are not grouped: old=%v,%v new=%v,%v", deletedY, oldSecond, newFirst, newSecond)
	}
	right := ed.r.X + ed.gutterW - 4 - ed.view.charWidth*0.5
	width := float32(ed.o.minDigits) * ed.view.charWidth
	for _, tc := range []struct {
		name string
		y    float32
	}{{"old first", deletedY}, {"old second", oldSecond}, {"new first", newFirst}, {"new second", newSecond}} {
		gutter := image.Rect(int(right-width), int(tc.y), int(right), int(tc.y+ed.view.lineHeight))
		text := image.Rect(int(ed.r.X+ed.gutterW+3), int(tc.y), int(ed.r.X+ed.gutterW+160), int(tc.y+ed.view.lineHeight))
		if darkPixels(tt.Image(), gutter) == 0 || darkPixels(tt.Image(), text) == 0 {
			t.Errorf("%s missing line number or content", tc.name)
		}
	}
	if d.Old.String() != old || d.New.String() != next {
		t.Fatal("viewer altered source contents")
	}
}

func darkPixels(img image.Image, bounds image.Rectangle) int {
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r+g+b < 3*65535*2/3 {
				count++
			}
		}
	}
	return count
}
