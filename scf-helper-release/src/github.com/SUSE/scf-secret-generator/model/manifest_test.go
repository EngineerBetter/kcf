package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManifestFileIsInvalid(t *testing.T) {
	t.Parallel()

	_, err := GetManifest(strings.NewReader("123123 123123"))

	assert.EqualError(t, err, "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `123123 ...` into model.Manifest")
}

func TestManifestConfigurationSectionNotFound(t *testing.T) {
	t.Parallel()

	_, err := GetManifest(strings.NewReader("roles: []"))

	assert.EqualError(t, err, "'configuration section' not found in manifest")
}
