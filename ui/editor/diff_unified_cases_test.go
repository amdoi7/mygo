package editor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestUnifiedShowsAuthoritativePatchHeader(t *testing.T) {
	patch, err := ParsePatch("@@ -10,1 +10,1 @@\n-old\n+new\n")
	if err != nil || len(patch) != 1 {
		t.Fatalf("patch=%v err=%v", patch, err)
	}
	d := &DiffState{Layout: DiffUnified}
	d.SetPatch(patch[0])
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	if _, ok := tt.Find("@@ -10,1 +10,1 @@"); !ok {
		t.Fatalf("patch header missing: %v", tt.Texts())
	}
}

func TestUnifiedGroupsPureAdditionsAndDeletions(t *testing.T) {
	for _, tc := range []struct {
		name, old, next string
		deleted         int
	}{
		{"added", "head\ntail", "head\nvalue = 1\nlimit = 2\ntail", 0},
		{"deleted", "head\nvalue = 1\nlimit = 2\ntail", "head\ntail", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &DiffState{Old: NewBuffer(tc.old), New: NewBuffer(tc.next), Layout: DiffUnified}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
			tt.Frame()
			deleted := 0
			for _, b := range d.sides[1].blks {
				if b.kind == blockDeleted {
					deleted++
				}
			}
			if deleted != tc.deleted {
				t.Fatalf("deleted blocks=%d, want %d", deleted, tc.deleted)
			}
		})
	}
}

func TestUnifiedGroupsCRLFWithoutChangingBytes(t *testing.T) {
	old, next := "head\r\nvalue = 1\r\nlimit = 2\r\ntail\r\n", "head\r\nvalue = 3\r\nlimit = 4\r\ntail\r\n"
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(next), Layout: DiffUnified, ExpandUnchanged: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	count := 0
	for _, b := range d.sides[1].blks {
		if b.kind == blockDeleted {
			count++
		}
	}
	var oldBytes, newBytes bytes.Buffer
	if _, err := d.Old.WriteTo(&oldBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := d.New.WriteTo(&newBytes); err != nil {
		t.Fatal(err)
	}
	if count != 1 || oldBytes.String() != old || newBytes.String() != next {
		t.Fatalf("blocks=%d old=%q new=%q", count, oldBytes.String(), newBytes.String())
	}
}

func TestUnifiedGroupsWrappedReplacementLines(t *testing.T) {
	old := "head\nvalue = \"" + strings.Repeat("before", 30) + "\"\nlimit = 1\ntail"
	next := "head\nvalue = \"" + strings.Repeat("after", 30) + "\"\nlimit = 2\ntail"
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(next), Layout: DiffUnified, Wrap: true, ExpandUnchanged: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 240, 500)
	tt.Frame()
	var deleted []block
	for _, b := range d.sides[1].blks {
		if b.kind == blockDeleted {
			deleted = append(deleted, b)
		}
	}
	if len(deleted) != 1 || deleted[0].from != 1 || deleted[0].to != 3 || deleted[0].height <= 2*d.eds[1].view.lineHeight {
		t.Fatalf("wrapped deleted blocks=%+v line height=%v", deleted, d.eds[1].view.lineHeight)
	}
}

func TestUnifiedGroupsReplacementVariantsWithoutTruncation(t *testing.T) {
	for _, tc := range []struct {
		name, old, next string
	}{
		{"unicode", "head\n用户 = 旧\n状态 = 旧\ntail", "head\n用户 = 新\n状态 = 新\ntail"},
		{"unequal", "head\nvalue = 1\nlimit = 2\ntail", "head\nvalue = 3\nextra = 5\nlimit = 4\ntail"},
		{"long", "head\nvalue = \"" + strings.Repeat("a", 1100) + "\"\nlimit = 1\ntail", "head\nvalue = \"" + strings.Repeat("b", 1100) + "\"\nlimit = 2\ntail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &DiffState{Old: NewBuffer(tc.old), New: NewBuffer(tc.next), Layout: DiffUnified, ExpandUnchanged: true}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
			tt.Frame()
			var deleted []block
			for _, b := range d.sides[1].blks {
				if b.kind == blockDeleted {
					deleted = append(deleted, b)
				}
			}
			if len(deleted) != 1 || deleted[0].from != 1 || deleted[0].to != 3 || d.Old.String() != tc.old || d.New.String() != tc.next {
				t.Fatalf("deleted blocks=%+v old=%q new=%q", deleted, d.Old.String(), d.New.String())
			}
		})
	}
}
