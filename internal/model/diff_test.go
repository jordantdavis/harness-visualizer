// internal/model/diff_test.go
package model

import (
	"reflect"
	"testing"
)

func TestDiffLines(t *testing.T) {
	got := DiffLines("a\nb\nc", "a\nB\nc")
	want := []DiffOp{
		{Kind: "context", Text: "a"},
		{Kind: "del", Text: "b"},
		{Kind: "add", Text: "B"},
		{Kind: "context", Text: "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiffLines mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestDiffLines_PureInsertion(t *testing.T) {
	got := DiffLines("a\nc", "a\nb\nc")
	want := []DiffOp{
		{Kind: "context", Text: "a"},
		{Kind: "add", Text: "b"},
		{Kind: "context", Text: "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiffLines mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestDiffLines_Identical(t *testing.T) {
	got := DiffLines("x\ny", "x\ny")
	want := []DiffOp{{Kind: "context", Text: "x"}, {Kind: "context", Text: "y"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
