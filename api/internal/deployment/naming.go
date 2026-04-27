package deployment

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
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

var (
	slugRandMu sync.Mutex
	slugRand   = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func GenerateSubDomainSlug() string {
	slugRandMu.Lock()
	defer slugRandMu.Unlock()

	return generateSubDomainSlug(slugRand)
}

func generateSubDomainSlug(r *rand.Rand) string {
	adj := adjectives[r.Intn(len(adjectives))]
	noun := nouns[r.Intn(len(nouns))]
	suffix := fmt.Sprintf("%04x", r.Intn(65536))
	return fmt.Sprintf("%s-%s-%s", adj, noun, suffix)
}
