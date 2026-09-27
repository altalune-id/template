package slug_test

import (
	"regexp"
	"testing"

	"altalune.id/template/internal/platform/slug"
)

var shape = regexp.MustCompile(`^[a-z]{3,8}-[a-z]{3,8}-[1-9][0-9]{3}$`)

func TestGenerate_Shape(t *testing.T) {
	for range 2000 {
		got := slug.Generate()
		if !shape.MatchString(got) {
			t.Fatalf("Generate() = %q, want <adjective>-<noun>-<four digits>", got)
		}
	}
}

func TestGenerate_Varies(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		seen[slug.Generate()] = true
	}
	if len(seen) < 100 {
		t.Fatalf("200 calls produced only %d distinct slugs", len(seen))
	}
}

func TestCombinations(t *testing.T) {
	if got := slug.Combinations(); got < 1_000_000 {
		t.Fatalf("Combinations() = %d, want at least 1e6 to keep collisions negligible", got)
	}
}
