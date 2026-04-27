package deployment

import (
	"math/rand"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSubDomainSlug(t *testing.T) {
	t.Parallel()

	slug := generateSubDomainSlug(rand.New(rand.NewSource(1)))

	assert.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9a-f]{4}$`), slug)
}
