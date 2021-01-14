package policyconfiguration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDockerReference tests DockerReferenceIdentity and DockerReferenceNamespaces simultaneously
// to ensure they are consistent.
func TestDockerReference(t *testing.T) {
	sha256Digest := "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	// Test both that DockerReferenceIdentity returns the expected value (fullName+suffix),
	// and that DockerReferenceNamespaces starts with the expected value (fullName), i.e. that the two functions are
	// consistent.
	for inputName, expectedNS := range map[string][]string{
		"example.com/ns/repo": {"example.com/ns/repo", "example.com/ns", "example.com", "*.com"},
		"example.com/repo":    {"example.com/repo", "example.com", "*.com"},
		"localhost/ns/repo":   {"localhost/ns/repo", "localhost/ns", "localhost"},
		// Note that "localhost" is special here: notlocalhost/repo is parsed as docker.io/notlocalhost.repo:
		"localhost/repo":                       {"localhost/repo", "localhost"},
		"notlocalhost/repo":                    {"docker.io/notlocalhost/repo", "docker.io/notlocalhost", "docker.io", "*.io"},
		"docker.io/ns/repo":                    {"docker.io/ns/repo", "docker.io/ns", "docker.io", "*.io"},
		"docker.io/library/repo":               {"docker.io/library/repo", "docker.io/library", "docker.io", "*.io"},
		"docker.io/repo":                       {"docker.io/library/repo", "docker.io/library", "docker.io", "*.io"},
		"ns/repo":                              {"docker.io/ns/repo", "docker.io/ns", "docker.io", "*.io"},
		"library/repo":                         {"docker.io/library/repo", "docker.io/library", "docker.io", "*.io"},
		"repo":                                 {"docker.io/library/repo", "docker.io/library", "docker.io", "*.io"},
		"yet.another.example.com:8443/ns/repo": {"yet.another.example.com:8443/ns/repo", "yet.another.example.com:8443/ns", "yet.another.example.com:8443", "*.another.example.com", "*.example.com", "*.com"},
	} {
		for inputSuffix, mappedSuffix := range map[string]string{
			":tag":       ":tag",
			sha256Digest: sha256Digest,
		} {
			fullInput := inputName + inputSuffix
			ref, err := reference.ParseNormalizedNamed(fullInput)
			require.NoError(t, err, fullInput)

			identity, err := DockerReferenceIdentity(ref)
			require.NoError(t, err, fullInput)
			assert.Equal(t, expectedNS[0]+mappedSuffix, identity, fullInput)

			ns := DockerReferenceNamespaces(ref)
			require.NotNil(t, ns, fullInput)
			require.Len(t, ns, len(expectedNS), fullInput)
			moreSpecific := identity
			for i := range expectedNS {
				assert.Equal(t, ns[i], expectedNS[i], fmt.Sprintf("%s item %d", fullInput, i))
				// Verify that expectedNS is ordered from most specific to least specific
				if strings.HasPrefix(ns[i], "*.") {
					// Check for subdomain matches if wildcard present
					assert.True(t, strings.Contains(moreSpecific, ns[i][1:]))
				} else {
					assert.True(t, strings.HasPrefix(moreSpecific, ns[i]))
				}
				moreSpecific = ns[i]
			}
		}
	}
}

func TestDockerReferenceIdentity(t *testing.T) {
	// TestDockerReference above has tested the core of the functionality, this tests only the failure cases.

	// Neither a tag nor digest
	parsed, err := reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	id, err := DockerReferenceIdentity(parsed)
	assert.Equal(t, "", id)
	assert.Error(t, err)

	// A github.com/distribution/reference value can have a tag and a digest at the same time!
	parsed, err = reference.ParseNormalizedNamed("busybox:notlatest@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	_, ok := parsed.(reference.Canonical)
	require.True(t, ok)
	_, ok = parsed.(reference.NamedTagged)
	require.True(t, ok)
	id, err = DockerReferenceIdentity(parsed)
	assert.Equal(t, "", id)
	assert.Error(t, err)
}
