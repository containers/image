package docker

// Many of these tests are made using github.com/dnaeon/go-vcr/recorder.
// See docker_client_test.go for more instructions.

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/dnaeon/go-vcr/recorder"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: Tests for quite a few methods.

// vcrImageDestination creates a dockerImageDestination using a series of HTTP request/response recordings
// using recordingBaseName.
// It returns the imageDestination and a cleanup callback
func vcrImageDestination(t *testing.T, ctx *types.SystemContext, recordingBaseName string, mode recorder.Mode,
	ref string) (*dockerImageDestination, func()) {
	ctx, httpWrapper, cleanup, dockerRef := prepareVCR(t, ctx, recordingBaseName, mode,
		ref)

	dest, err := newImageDestination(ctx, dockerRef, httpWrapper)
	require.NoError(t, err)
	return dest, cleanup
}

// See the comment above TestDockerClientGetExtensionsSignatures for instructions on setting up the recording.
func TestDockerImageDestinationPutSignaturesToAPIExtension(t *testing.T) {
	ctx := &types.SystemContext{
		DockerAuthConfig: &types.DockerAuthConfig{
			Username: "unused",
			Password: "dh2juhu6LbGYGSHKMUa5BFEpyoPMYDVA59hxd3FCfbU",
		},
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
	}
	expectedSignature1, err := ioutil.ReadFile("fixtures/extension-personal-personal.signature")
	require.NoError(t, err)

	// Success
	dest, cleanup := vcrImageDestination(t, ctx, "putSignaturesToAPIExtension-success", recorder.ModeReplaying,
		"//localhost:5000/myns/personal:personal")
	defer cleanup()
	// The value can be obtained e.g. from (oc get istag personal:personal) value image.dockerImageReference in-container.
	manifestDigest := digest.Digest("sha256:8d7fe3e157e56648ab790794970fbdfe82c84af79e807443b98df92c822a9b9b")
	sig2 := []byte("This is not really a signature")
	err = dest.putSignaturesToAPIExtension(context.Background(), [][]byte{sig2}, manifestDigest)
	require.NoError(t, err)
	// Verify that this preserves the original signature and creates a new one.
	esl, err := dest.c.getExtensionsSignatures(context.Background(), dest.ref, manifestDigest)
	require.NoError(t, err)
	// We do not know what extensionSignature.Name has been randomly generated,
	// so only verify that it has the expected format, and then replace it for the purposes of equality comparison.
	require.Len(t, esl.Signatures, 2)
	assert.Regexp(t, manifestDigest.String()+"@.{32}", esl.Signatures[1].Name)
	assert.Equal(t, &extensionSignatureList{
		Signatures: []extensionSignature{
			{
				Version: extensionSignatureSchemaVersion,
				Name:    manifestDigest.String() + "@809439d23da88df57186b0f2fce91e9a",
				Type:    extensionSignatureTypeAtomic,
				Content: expectedSignature1,
			},
			{
				Version: extensionSignatureSchemaVersion,
				Name:    esl.Signatures[1].Name, // This is comparing the value with itself, i.e. ignoring the comparison; we have checked the format above.
				Type:    extensionSignatureTypeAtomic,
				Content: sig2,
			},
		},
	}, esl)

	// TODO? Test that unknown signature kinds are silently ignored.
	// TODO? Test the various failure modes.
}
