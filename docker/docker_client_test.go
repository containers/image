package docker

// Many of these tests are made using github.com/dnaeon/go-vcr/recorder, and under ordinary
// operation are expected to run completely off-line from the recorded interactions.
//
// To update an individual test, set up a registry server as needed (usually just
// allow access to docker.io; special setup will be described with individual tests),
// temporarily edit the test to use recorder.ModeRecording, run the test.
// Don’t forget to revert to recorder.ModeReplaying!

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/image/v5/types"
	"github.com/dnaeon/go-vcr/recorder"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerCertDir(t *testing.T) {
	const nondefaultFullPath = "/this/is/not/the/default/full/path"
	const nondefaultPerHostDir = "/this/is/not/the/default/certs.d"
	const variableReference = "$HOME"
	const rootPrefix = "/root/prefix"
	const registryHostPort = "thishostdefinitelydoesnotexist:5000"

	systemPerHostResult := filepath.Join(perHostCertDirs[len(perHostCertDirs)-1].path, registryHostPort)
	for _, c := range []struct {
		sys      *types.SystemContext
		expected string
	}{
		// The common case
		{nil, systemPerHostResult},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, systemPerHostResult},
		// Full path overridden
		{&types.SystemContext{DockerCertPath: nondefaultFullPath}, nondefaultFullPath},
		// Per-host path overridden
		{
			&types.SystemContext{DockerPerHostCertDirPath: nondefaultPerHostDir},
			filepath.Join(nondefaultPerHostDir, registryHostPort),
		},
		// Both overridden
		{
			&types.SystemContext{
				DockerCertPath:           nondefaultFullPath,
				DockerPerHostCertDirPath: nondefaultPerHostDir,
			},
			nondefaultFullPath,
		},
		// Root overridden
		{
			&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix},
			filepath.Join(rootPrefix, systemPerHostResult),
		},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				DockerCertPath:               nondefaultFullPath,
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			nondefaultFullPath,
		},
		{
			&types.SystemContext{
				DockerPerHostCertDirPath:     nondefaultPerHostDir,
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			filepath.Join(nondefaultPerHostDir, registryHostPort),
		},
		// … and everything at once
		{
			&types.SystemContext{
				DockerCertPath:               nondefaultFullPath,
				DockerPerHostCertDirPath:     nondefaultPerHostDir,
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			nondefaultFullPath,
		},
		// No environment expansion happens in the overridden paths
		{&types.SystemContext{DockerCertPath: variableReference}, variableReference},
		{
			&types.SystemContext{DockerPerHostCertDirPath: variableReference},
			filepath.Join(variableReference, registryHostPort),
		},
	} {
		path, err := dockerCertDir(c.sys, registryHostPort)
		require.Equal(t, nil, err)
		assert.Equal(t, c.expected, path)
	}
}

func TestNewBearerTokenFromJsonBlob(t *testing.T) {
	expected := &bearerToken{Token: "IAmAToken", ExpiresIn: 100, IssuedAt: time.Unix(1514800802, 0)}
	tokenBlob := []byte(`{"token":"IAmAToken","expires_in":100,"issued_at":"2018-01-01T10:00:02+00:00"}`)
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertBearerTokensEqual(t, expected, token)
}

func TestNewBearerAccessTokenFromJsonBlob(t *testing.T) {
	expected := &bearerToken{Token: "IAmAToken", ExpiresIn: 100, IssuedAt: time.Unix(1514800802, 0)}
	tokenBlob := []byte(`{"access_token":"IAmAToken","expires_in":100,"issued_at":"2018-01-01T10:00:02+00:00"}`)
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertBearerTokensEqual(t, expected, token)
}

func TestNewBearerTokenFromInvalidJsonBlob(t *testing.T) {
	tokenBlob := []byte("IAmNotJson")
	_, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err == nil {
		t.Fatalf("unexpected an error unmarshalling JSON")
	}
}

func TestNewBearerTokenSmallExpiryFromJsonBlob(t *testing.T) {
	expected := &bearerToken{Token: "IAmAToken", ExpiresIn: 60, IssuedAt: time.Unix(1514800802, 0)}
	tokenBlob := []byte(`{"token":"IAmAToken","expires_in":1,"issued_at":"2018-01-01T10:00:02+00:00"}`)
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertBearerTokensEqual(t, expected, token)
}

func TestNewBearerTokenIssuedAtZeroFromJsonBlob(t *testing.T) {
	zeroTime := time.Time{}.Format(time.RFC3339)
	now := time.Now()
	tokenBlob := []byte(fmt.Sprintf(`{"token":"IAmAToken","expires_in":100,"issued_at":"%s"}`, zeroTime))
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token.IssuedAt.Before(now) {
		t.Fatalf("expected [%s] not to be before [%s]", token.IssuedAt, now)
	}

}

func assertBearerTokensEqual(t *testing.T, expected, subject *bearerToken) {
	if expected.Token != subject.Token {
		t.Fatalf("expected [%s] to equal [%s], it did not", subject.Token, expected.Token)
	}
	if expected.ExpiresIn != subject.ExpiresIn {
		t.Fatalf("expected [%d] to equal [%d], it did not", subject.ExpiresIn, expected.ExpiresIn)
	}
	if !expected.IssuedAt.Equal(subject.IssuedAt) {
		t.Fatalf("expected [%s] to equal [%s], it did not", subject.IssuedAt, expected.IssuedAt)
	}
}

// prepareVCR is a shared helper for setting up HTTP request/response recordings using recordingBaseName.
// It returns the result of preparing ctx and ref, a httpWrapper and a cleanup callback.
func prepareVCR(t *testing.T, ctx *types.SystemContext, recordingBaseName string, mode recorder.Mode,
	ref string) (*types.SystemContext, httpWrapper, func(), dockerReference) {
	// Always set ctx.DockerAuthConfig so that we don’t depend on $HOME.
	ourCtx := types.SystemContext{}
	if ctx != nil {
		ourCtx = *ctx
	}
	if ourCtx.DockerAuthConfig == nil {
		ourCtx.DockerAuthConfig = &types.DockerAuthConfig{}
	}

	parsedRef, err := ParseReference(ref)
	require.NoError(t, err)
	dockerRef, ok := parsedRef.(dockerReference)
	require.True(t, ok)

	// dockerClient creates a new http.Client in each call to getBearerToken, so we need
	// not just a single recording with a given name, but a sequence of recordings.
	recorderNo := 0
	allRecorders := []*recorder.Recorder{}
	httpWrapper := func(rt http.RoundTripper) http.RoundTripper {
		recordingName := fmt.Sprintf("fixtures/recording-%s-%d", recordingBaseName, recorderNo)
		recorderNo++

		// Always create the file first; without that, even with mode == recorder.ModeReplaying,
		// the recorder is in recording mode.  We want to ensure that recording happens only
		// as an intentional decision.
		recordingFileName := recordingName + ".yaml"
		f, err := os.OpenFile(recordingFileName, os.O_RDWR|os.O_CREATE, 0600)
		require.NoError(t, err)
		f.Close()

		r, err := recorder.NewAsMode(recordingName, mode, rt)
		require.NoError(t, err)
		allRecorders = append(allRecorders, r)
		return r
	}

	closeRecorders := func() {
		for _, r := range allRecorders {
			err := r.Stop()
			require.NoError(t, err)
		}
	}

	return &ourCtx, httpWrapper, closeRecorders, dockerRef
}

// vcrDockerClient creates a dockerClient using a series of HTTP request/response recordings
// using recordingBaseName.
// It returns a dockerClient and a cleanup callback, and the parsed version of ref.
func vcrDockerClient(t *testing.T, ctx *types.SystemContext, recordingBaseName string, mode recorder.Mode,
	ref string, write bool, actions string) (*dockerClient, func(), dockerReference) {
	ctx, httpWrapper, cleanup, dockerRef := prepareVCR(t, ctx, recordingBaseName, mode,
		ref)

	client, err := newDockerClientFromRef(ctx, dockerRef, write, actions, httpWrapper)
	require.NoError(t, err)
	return client, cleanup, dockerRef
}

// To record the the X-Registry-Supports-Signatures tests,
// use skopeo's integration tests to set up an Atomic Registry per https://github.com/projectatomic/skopeo/pull/320
// except running the container with -p 5000:5000, e.g.
// (sudo docker run --rm -i  -t -p 5000:5000 "skopeo-dev:openshift-shell" bash)
// Then set:
// - the username:password values obtained by decoding "auth" from the in-container  ~/.docker/config.json
// - the manifest digest reference e.g. from (oc get istag personal:personal) value image.dockerImageReference in-container.
// - the signature name from the same (oc get istag personal:personal)
func TestDockerClientGetExtensionsSignatures(t *testing.T) {
	ctx := &types.SystemContext{
		DockerAuthConfig: &types.DockerAuthConfig{
			Username: "unused",
			Password: "dh2juhu6LbGYGSHKMUa5BFEpyoPMYDVA59hxd3FCfbU",
		},
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
	}

	// Success
	manifestDigest := digest.Digest("sha256:8d7fe3e157e56648ab790794970fbdfe82c84af79e807443b98df92c822a9b9b")
	client, cleanup, dockerRef := vcrDockerClient(t, ctx, "getExtensionsSignatures-success", recorder.ModeReplaying,
		"//localhost:5000/myns/personal:personal", false, "pull")
	defer cleanup()
	esl, err := client.getExtensionsSignatures(context.Background(), dockerRef, manifestDigest)
	require.NoError(t, err)
	expectedSignature, err := ioutil.ReadFile("fixtures/extension-personal-personal.signature")
	require.NoError(t, err)
	assert.Equal(t, &extensionSignatureList{
		Signatures: []extensionSignature{{
			Version: extensionSignatureSchemaVersion,
			Name:    manifestDigest.String() + "@809439d23da88df57186b0f2fce91e9a",
			Type:    extensionSignatureTypeAtomic,
			Content: expectedSignature,
		}},
	}, esl)

	// TODO? Test the various failure modes.
}
