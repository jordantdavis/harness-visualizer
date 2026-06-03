package cli

import (
	"bytes"
	"testing"
	"time"
)

// BenchmarkHookExecute locks in the cobra startup cost on the hook hot path. It
// measures the in-process path only (the seam is a no-op), excluding the real
// network POST, so a future cobra upgrade or our own bloat can't quietly
// regress the per-event budget. See TestHookExecuteUnderBudget for the hard
// ceiling that fails CI.
func BenchmarkHookExecute(b *testing.B) {
	orig := hookRun
	hookRun = func(args []string) int { return 0 }
	b.Cleanup(func() { hookRun = orig })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root := newRootCmd()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		runRoot(root, []string{"hook"}, true)
	}
}

// TestHookExecuteUnderBudget guards the in-process hook dispatch against
// regression past a small fraction of the 100ms per-event budget. The real
// cobra build + dispatch is sub-millisecond; the ceiling here is generous to
// avoid CI flakiness on loaded machines while still catching gross regressions.
func TestHookExecuteUnderBudget(t *testing.T) {
	const budget = 25 * time.Millisecond

	orig := hookRun
	hookRun = func(args []string) int { return 0 }
	t.Cleanup(func() { hookRun = orig })

	// Warm up once (first call pays one-time init), then measure the steady state.
	for i := 0; i < 3; i++ {
		root := newRootCmd()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		runRoot(root, []string{"hook"}, true)
	}

	start := time.Now()
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	runRoot(root, []string{"hook"}, true)
	if elapsed := time.Since(start); elapsed > budget {
		t.Fatalf("hook in-process dispatch took %v, exceeds budget %v", elapsed, budget)
	}
}
