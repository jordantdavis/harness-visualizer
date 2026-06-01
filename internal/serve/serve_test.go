package serve

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRun_OpensResolvedURL(t *testing.T) {
	var opened string
	code := run(
		func() (string, error) { return "127.0.0.1:7842", nil },
		func(u string) error { opened = u; return nil },
		&bytes.Buffer{},
	)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
	if opened != "http://127.0.0.1:7842/" {
		t.Fatalf("opened=%q, want http://127.0.0.1:7842/", opened)
	}
}

func TestRun_EnsureFailureReturns1(t *testing.T) {
	code := run(
		func() (string, error) { return "", errors.New("nope") },
		func(string) error { return nil },
		&bytes.Buffer{},
	)
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
}

func TestRun_OpenFailureStillSucceeds(t *testing.T) {
	out := &bytes.Buffer{}
	code := run(
		func() (string, error) { return "127.0.0.1:9000", nil },
		func(string) error { return errors.New("no browser") },
		out,
	)
	if code != 0 {
		t.Fatalf("code=%d, want 0 (open failure is non-fatal)", code)
	}
	if !strings.Contains(out.String(), "127.0.0.1:9000") {
		t.Fatalf("out=%q, want it to print the URL for manual opening", out.String())
	}
}
