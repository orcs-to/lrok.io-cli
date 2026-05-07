// Package names generates friendly subdomain candidates of the shape
// `<adjective>-<animal>`. Used by `lrok reserve` when the requested name
// is taken: the CLI proposes 3 alternatives instead of just printing the
// error.
//
// Pool sizing: 60 adjectives × 60 animals = 3600 distinct base names —
// big enough that a 3-shot suggestion against a few hundred reserved
// names is virtually never going to collide with itself, small enough
// to keep the binary tiny.

package names

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

var adjectives = []string{
	"amber", "brave", "calm", "cosmic", "crimson", "curious", "dusty",
	"eager", "electric", "fancy", "feisty", "floppy", "frosty", "gentle",
	"glassy", "golden", "happy", "hidden", "humble", "jolly", "kind",
	"lively", "loud", "lucky", "merry", "mighty", "misty", "noble",
	"odd", "olive", "patient", "plucky", "polished", "proud", "quick",
	"quiet", "rapid", "rusty", "shiny", "silent", "silky", "smooth",
	"soft", "sparkly", "spicy", "spirited", "steady", "sturdy", "sunny",
	"swift", "tame", "tidy", "tiny", "vivid", "wandering", "warm",
	"whimsical", "wise", "witty", "zesty",
}

var animals = []string{
	"axolotl", "badger", "bear", "beaver", "bee", "buffalo", "camel",
	"cat", "cheetah", "cobra", "condor", "coyote", "crane", "crow",
	"dolphin", "dove", "eagle", "elephant", "ferret", "fox", "gazelle",
	"goose", "gopher", "grouse", "hawk", "hedgehog", "heron", "hyena",
	"ibex", "iguana", "jaguar", "kangaroo", "koala", "lemur", "leopard",
	"lion", "llama", "lynx", "macaw", "marmot", "moose", "mouse",
	"narwhal", "newt", "ocelot", "octopus", "ostrich", "otter", "owl",
	"panda", "panther", "parrot", "penguin", "puma", "raccoon", "raven",
	"rhino", "salmon", "seal", "shark", "sloth", "swan", "tiger", "toad",
	"trout", "turtle", "viper", "walrus", "weasel", "whale", "wolf",
	"wombat", "yak", "zebra",
}

// Suggest returns up to n unique friendly names. It uses crypto/rand
// for a small splash of seasonal variety; the names are aesthetic, not
// security-sensitive, but rolling math/rand without a seed gives the
// same suggestion list every time which is bad UX.
func Suggest(n int) []string {
	if n <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, n)
	out := make([]string, 0, n)
	// Bound the loop so a freak collision storm doesn't spin forever.
	for i := 0; i < n*10 && len(out) < n; i++ {
		a := adjectives[randIndex(len(adjectives))]
		x := animals[randIndex(len(animals))]
		name := fmt.Sprintf("%s-%s", a, x)
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// randIndex picks a uniform index in [0, n). Falls back to a fixed
// index if crypto/rand is somehow unavailable — this is a name picker,
// not a CSPRNG-critical path.
func randIndex(n int) int {
	if n <= 0 {
		return 0
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(binary.BigEndian.Uint32(b[:])) % n
}
