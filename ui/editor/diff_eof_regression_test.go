package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Regression: upstream mygo v0.6.4 paints the virtual EOF as a deleted line.
func TestUnifiedDeletionDoesNotPaintVirtualEOF(t *testing.T) {
	for _, old := range []string{"old\n", "old", "\n"} {
		t.Run(old, func(t *testing.T) {
			d := &DiffState{Old: NewBuffer(old), New: NewBuffer(""), Layout: DiffUnified}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
			tt.Frame()
			var deleted []block
			for _, b := range d.sides[1].blks {
				if b.kind == blockDeleted {
					deleted = append(deleted, b)
				}
			}
			if len(deleted) != 1 || deleted[0].from != 0 || deleted[0].to != 1 {
				t.Fatalf("old=%q deleted=%+v", old, deleted)
			}
		})
	}
}
