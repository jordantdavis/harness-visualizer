package tui

import "testing"

// TestChooseLayout verifies the layout selection thresholds.
// Phase 8: Wide and Medium are removed; ≥80 cols = LayoutBrowse, <80 = LayoutNarrow.
func TestChooseLayout(t *testing.T) {
	tests := []struct {
		cols, rows int
		want       Layout
	}{
		{cols: 30, rows: 5, want: LayoutTooSmall},   // too small both dims
		{cols: 30, rows: 20, want: LayoutTooSmall},  // too narrow
		{cols: 80, rows: 5, want: LayoutTooSmall},   // too short
		{cols: 40, rows: 10, want: LayoutNarrow},    // minimum accepted
		{cols: 79, rows: 24, want: LayoutNarrow},    // just below Browse threshold
		{cols: 80, rows: 24, want: LayoutBrowse},    // exactly at Browse threshold
		{cols: 119, rows: 24, want: LayoutBrowse},   // medium-old range → Browse
		{cols: 120, rows: 24, want: LayoutBrowse},   // old Wide range → Browse
		{cols: 200, rows: 50, want: LayoutBrowse},   // well above threshold
	}
	for _, tc := range tests {
		got := chooseLayout(tc.cols, tc.rows)
		if got != tc.want {
			t.Errorf("chooseLayout(%d, %d) = %v, want %v", tc.cols, tc.rows, got, tc.want)
		}
	}
}

// TestPaneWidthsBrowse verifies Browse pane widths: sessions=36, events fills remainder.
func TestPaneWidthsBrowse(t *testing.T) {
	const total = 120
	s, e, _ := paneWidths(LayoutBrowse, total)
	if s != 36 {
		t.Errorf("Browse sessions width = %d, want 36", s)
	}
	// events = total - sessions(36) - divider(1)
	wantE := total - 36 - 1
	if e != wantE {
		t.Errorf("Browse events width = %d, want %d", e, wantE)
	}
	// sum check
	if s+e != total-1 {
		t.Errorf("Browse widths %d+%d = %d, want %d", s, e, s+e, total-1)
	}
}

// TestPaneWidthsDrill verifies Drill pane widths: inspector=46, events fills remainder.
func TestPaneWidthsDrill(t *testing.T) {
	const total = 120
	e, i := paneWidthsDrill(total)
	if i != 46 {
		t.Errorf("Drill inspector width = %d, want 46", i)
	}
	wantE := total - 46 - 1
	if e != wantE {
		t.Errorf("Drill events width = %d, want %d", e, wantE)
	}
}

// TestPaneWidthsNarrow verifies all Narrow pane widths equal totalCols.
func TestPaneWidthsNarrow(t *testing.T) {
	s, e, i := paneWidths(LayoutNarrow, 70)
	if s != 70 || e != 70 || i != 70 {
		t.Errorf("narrow widths should all be totalCols, got %d %d %d", s, e, i)
	}
}

// TestLayoutConstantsAlias verifies that LayoutWide and LayoutMedium are aliases
// for LayoutBrowse (backward compat for tests that still use the old names).
func TestLayoutConstantsAlias(t *testing.T) {
	if LayoutWide != LayoutBrowse {
		t.Error("LayoutWide must equal LayoutBrowse")
	}
	if LayoutMedium != LayoutBrowse {
		t.Error("LayoutMedium must equal LayoutBrowse")
	}
}
