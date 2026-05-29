package tui

import "testing"

func TestChooseLayout(t *testing.T) {
	tests := []struct {
		cols, rows int
		want       Layout
	}{
		{cols: 30, rows: 5, want: LayoutTooSmall},   // too small both dims
		{cols: 30, rows: 20, want: LayoutTooSmall},  // too narrow
		{cols: 80, rows: 5, want: LayoutTooSmall},   // too short
		{cols: 40, rows: 10, want: LayoutNarrow},    // minimum accepted
		{cols: 79, rows: 24, want: LayoutNarrow},    // just below medium threshold
		{cols: 80, rows: 24, want: LayoutMedium},    // exactly medium
		{cols: 119, rows: 24, want: LayoutMedium},   // just below wide threshold
		{cols: 120, rows: 24, want: LayoutWide},     // exactly wide
		{cols: 200, rows: 50, want: LayoutWide},     // well above wide
	}
	for _, tc := range tests {
		got := chooseLayout(tc.cols, tc.rows)
		if got != tc.want {
			t.Errorf("chooseLayout(%d, %d) = %v, want %v", tc.cols, tc.rows, got, tc.want)
		}
	}
}

func TestPaneWidthsWide(t *testing.T) {
	s, e, i := paneWidths(LayoutWide, 160)
	if s != 22 {
		t.Errorf("sessions width = %d, want 22", s)
	}
	if i != 30 {
		t.Errorf("inspector width = %d, want 30", i)
	}
	// 160 - 22 - 30 - 2 dividers = 106
	if e != 106 {
		t.Errorf("events width = %d, want 106", e)
	}
	// Must sum to total - dividers.
	if s+e+i != 160-2 {
		t.Errorf("widths %d+%d+%d = %d, want %d", s, e, i, s+e+i, 160-2)
	}
}

func TestPaneWidthsMedium(t *testing.T) {
	s, e, _ := paneWidths(LayoutMedium, 100)
	if s != 22 {
		t.Errorf("sessions width = %d, want 22", s)
	}
	// 100 - 22 - 1 divider = 77
	if e != 77 {
		t.Errorf("events width = %d, want 77", e)
	}
}

func TestPaneWidthsNarrow(t *testing.T) {
	s, e, i := paneWidths(LayoutNarrow, 70)
	if s != 70 || e != 70 || i != 70 {
		t.Errorf("narrow widths should all be totalCols, got %d %d %d", s, e, i)
	}
}
