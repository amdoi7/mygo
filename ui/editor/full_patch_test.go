package editor

import (
	"fmt"
	"testing"
)

func TestFullPatchPreservesAuthoritativeChangesAndContextFolding(t *testing.T) {
	patch := FilePatch{OldName: "file", NewName: "file"}
	hunk := PatchHunk{OldStart: 1, NewStart: 1, OldLines: 70, NewLines: 70}
	for line := range 70 {
		text := fmt.Sprintf("value%02d", line)
		if line == 30 {
			hunk.Lines = append(hunk.Lines, "-"+text, "+"+text)
		} else {
			hunk.Lines = append(hunk.Lines, " "+text)
		}
	}
	patch.Hunks = append(patch.Hunks, hunk)
	state := &DiffState{}
	state.SetFullPatch(patch)
	if added, removed := state.Stats(); added != 1 || removed != 1 {
		t.Fatalf("supplied changes were recomputed: added=%d removed=%d", added, removed)
	}
	if state.partial != nil {
		t.Fatal("full patch retained partial-context presentation")
	}
	gaps := state.gaps(state.Old.LineCount(), state.New.LineCount())
	if len(gaps) == 0 || gaps[0].hidden != 27 {
		t.Fatalf("context did not fold around supplied changes: %+v", gaps)
	}
}
