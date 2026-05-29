package tui

import (
	"testing"
	"time"
)

// TestTokensExist verifies the theme exports the expected semantic token set.
func TestTokensExist(t *testing.T) {
	tok := defaultTheme()
	// None of the tokens should panic or be nil; just access them.
	_ = tok.success
	_ = tok.failure
	_ = tok.running
	_ = tok.info
	_ = tok.muted
	_ = tok.selection
	_ = tok.header
}

// TestNoColorTokensAreEmpty verifies that noColor mode produces no ANSI styling.
// We don't require a specific representation, only that rendering a string with
// a noColor token does NOT produce ANSI escape sequences.
func TestNoColorTokensAreEmpty(t *testing.T) {
	tok := noColorTheme()
	rendered := tok.success.Render("hello")
	for _, ch := range rendered {
		if ch == '\x1b' {
			t.Errorf("noColor theme must not emit ANSI escapes, got %q", rendered)
			return
		}
	}
}

// TestReducedMotionLiveStatusStatic asserts that when reducedMotion is set,
// the live status text contains NO animated block character (▮) and still
// includes the rate/idle info.
func TestReducedMotionLiveStatusStatic(t *testing.T) {
	m := liveModel()
	m.reducedMotion = true
	m.streamUp = true
	now := m.now()
	// Simulate recent events so the rate path is exercised.
	m.recentEvents = []time.Time{
		now.Add(-500 * time.Millisecond),
		now.Add(-200 * time.Millisecond),
		now,
	}
	m.lastEventAt = now

	s := m.liveStatusText()
	if s == "" {
		t.Fatal("reducedMotion: liveStatusText must not be empty while streaming")
	}
	// No animated block character.
	if contains(s, "▮") {
		t.Errorf("reducedMotion: animated ▮ must not appear in status, got %q", s)
	}
	// Must still carry rate info (some digit + /s).
	if !contains(s, "/s") {
		t.Errorf("reducedMotion: rate must still appear in status, got %q", s)
	}
}

// TestReducedMotionIdleTextUnchanged asserts that idle text is preserved
// (no animation to strip there anyway).
func TestReducedMotionIdleTextUnchanged(t *testing.T) {
	m := liveModel()
	m.reducedMotion = true
	m.streamUp = true
	m.lastEventAt = m.now().Add(-7 * time.Second)

	s := m.liveStatusText()
	if !contains(s, "idle") {
		t.Errorf("reducedMotion idle: expected 'idle' in status, got %q", s)
	}
}

// TestNewModelReducedMotionDefault verifies reducedMotion starts false.
func TestNewModelReducedMotionDefault(t *testing.T) {
	m := newModel(fixtureClient(), false, false)
	if m.reducedMotion {
		t.Error("reducedMotion should default to false")
	}
}

// TestNewModelNoColorImpliesReducedMotion verifies noColor mode sets reducedMotion.
func TestNewModelNoColorImpliesReducedMotion(t *testing.T) {
	m := newModel(fixtureClient(), true, false)
	if !m.reducedMotion {
		t.Error("noColor=true should imply reducedMotion=true")
	}
}
