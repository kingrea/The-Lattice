package logbook

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTailReturnsRecentLinesAndTotal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journey.log")
	book, err := New(path)
	if err != nil {
		t.Fatalf("new logbook: %v", err)
	}
	for i := 0; i < 5; i++ {
		book.Info("entry-%d", i)
	}
	lines, total := book.Tail(3)
	if total != 5 {
		t.Fatalf("total lines = %d, want 5", total)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	for idx, want := range []string{"entry-2", "entry-3", "entry-4"} {
		if !strings.Contains(lines[idx], want) {
			t.Fatalf("line %d = %q, missing %s", idx, lines[idx], want)
		}
	}
}
