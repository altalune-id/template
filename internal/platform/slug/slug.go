// Package slug generates human-pronounceable, URL-safe default slugs.
package slug

import (
	"math/rand/v2"
	"strconv"
	"strings"
)

const (
	suffixMin = 1000
	suffixMax = 9999
)

//nolint:gochecknoglobals // immutable word list, not runtime state.
var adjectives = [...]string{
	"amber", "ancient", "autumn", "bold", "brave", "breezy", "bright", "calm",
	"clear", "cool", "cosmic", "crisp", "curly", "deep", "dusty", "eager",
	"early", "fancy", "fresh", "gentle", "glossy", "golden", "green", "happy",
	"hidden", "jolly", "keen", "lively", "lunar", "mellow", "merry", "misty",
	"neat", "noble", "polar", "proud", "quiet", "rapid", "rustic", "silent",
	"silver", "snowy", "solar", "spry", "sunny", "tidy", "warm", "wispy",
}

//nolint:gochecknoglobals // immutable word list, not runtime state.
var nouns = [...]string{
	"anchor", "arbor", "beacon", "birch", "bloom", "breeze", "brook", "canyon",
	"cedar", "cliff", "cloud", "comet", "coral", "cove", "creek", "delta",
	"dune", "ember", "fern", "field", "forest", "frost", "garden", "glade",
	"glen", "grove", "harbor", "haven", "hill", "island", "lagoon", "lake",
	"leaf", "meadow", "mesa", "moss", "oasis", "orbit", "pine", "prairie",
	"reef", "ridge", "river", "shore", "spruce", "summit", "thicket", "valley",
}

// Generate returns a slug shaped "<adjective>-<noun>-<four digits>", such as "wispy-frost-4821".
func Generate() string {
	var b strings.Builder
	b.WriteString(adjectives[rand.IntN(len(adjectives))])
	b.WriteByte('-')
	b.WriteString(nouns[rand.IntN(len(nouns))])
	b.WriteByte('-')
	b.WriteString(strconv.Itoa(suffixMin + rand.IntN(suffixMax-suffixMin+1)))
	return b.String()
}

// Combinations reports how many distinct slugs Generate can produce.
func Combinations() int {
	return len(adjectives) * len(nouns) * (suffixMax - suffixMin + 1)
}
