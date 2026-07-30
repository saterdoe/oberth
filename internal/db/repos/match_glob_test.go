package repos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchGlob_ExactMatch(t *testing.T) {
	assert.True(t, matchGlob("foo/bar", "foo/bar"))
}

func TestMatchGlob_ExactMismatch(t *testing.T) {
	assert.False(t, matchGlob("foo/bar", "foo/baz"))
}

func TestMatchGlob_EmptyPattern(t *testing.T) {
	assert.True(t, matchGlob("", "anything"))
}

func TestMatchGlob_WildcardOnly(t *testing.T) {
	assert.True(t, matchGlob("*", "anything"))
}

func TestMatchGlob_PrefixWildcard(t *testing.T) {
	assert.True(t, matchGlob("*.go", "main.go"))
	assert.False(t, matchGlob("*.go", "main.ts"))
}

func TestMatchGlob_SuffixWildcard(t *testing.T) {
	assert.True(t, matchGlob("src/*", "src/main.go"))
	assert.False(t, matchGlob("src/*", "lib/main.go"))
}

func TestMatchGlob_ContainsWildcard(t *testing.T) {
	assert.True(t, matchGlob("*middle*", "src/middle/ware"))
	assert.True(t, matchGlob("*middle*", "middle"))
	assert.False(t, matchGlob("*middle*", "src/outer"))
}

func TestMatchGlob_SubdirectoryPattern(t *testing.T) {
	assert.True(t, matchGlob("frontend/*", "frontend/src/app.tsx"))
	assert.False(t, matchGlob("frontend/*", "backend/src/app.tsx"))
}

func TestMatchGlob_FileExtensionPattern(t *testing.T) {
	assert.True(t, matchGlob("*.test.ts", "component.test.ts"))
	assert.False(t, matchGlob("*.test.ts", "component.ts"))
}
