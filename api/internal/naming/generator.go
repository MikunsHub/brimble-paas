package naming

import (
	"fmt"
	"math/rand"
)

var adjectives = []string{
	"happy", "swift", "bold", "calm", "eager", "brave", "clever",
	"bright", "cool", "crisp", "dazzling", "elegant", "fierce",
	"gentle", "honest", "jolly", "keen", "lively", "merry", "noble",
	"proud", "quiet", "rapid", "sharp", "tidy", "unique", "vivid",
	"warm", "witty", "zesty",
}

var nouns = []string{
	"cat", "fox", "river", "hawk", "storm", "wolf", "bear",
	"cloud", "delta", "ember", "fjord", "grove", "hill", "isle",
	"jewel", "knoll", "lake", "mesa", "nest", "oak", "peak",
	"quest", "reef", "sage", "tide", "vale", "wind", "yarn",
}

func GenerateSubDomainSlug() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	suffix := fmt.Sprintf("%04x", rand.Intn(65536))
	return fmt.Sprintf("%s-%s-%s", adj, noun, suffix)
}
