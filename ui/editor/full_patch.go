package editor

// SetFullPatch consumes a patch containing every line of both files. Context
// folding stays available, and the supplied changes are not recomputed.
func (d *DiffState) SetFullPatch(p FilePatch) {
	d.SetPatch(p)
	d.partial = nil
}
