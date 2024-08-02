package signature

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigstoreFromComponents(t *testing.T) {
	const mimeType = "mime-type"
	payload := []byte("payload")
	annotations := map[string]string{"a": "b", "c": "d"}

	sig := SigstoreFromComponents(mimeType, payload, annotations)
	assert.Equal(t, Sigstore{
		untrustedMIMEType:    mimeType,
		untrustedPayload:     payload,
		untrustedAnnotations: annotations,
	}, sig)
}

func TestSigstoreFromBlobChunk(t *testing.T) {
	// Success
	json := []byte(`{"mimeType":"mime-type","payload":"cGF5bG9hZA==", "annotations":{"a":"b","c":"d"}}`)
	res, err := sigstoreFromBlobChunk(json)
	require.NoError(t, err)
	assert.Equal(t, "mime-type", res.UntrustedMIMEType())
	assert.Equal(t, []byte("payload"), res.UntrustedPayload())
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, res.UntrustedAnnotations())

	// Invalid JSON
	_, err = sigstoreFromBlobChunk([]byte("&"))
	assert.Error(t, err)
}

func TestSigstoreFormatID(t *testing.T) {
	sig := SigstoreFromComponents("mime-type", []byte("payload"),
		map[string]string{"a": "b", "c": "d"})
	assert.Equal(t, SigstoreFormat, sig.FormatID())
}

func TestSigstoreBlobChunk(t *testing.T) {
	sig := SigstoreFromComponents("mime-type", []byte("payload"),
		map[string]string{"a": "b", "c": "d"})
	res, err := sig.blobChunk()
	require.NoError(t, err)

	expectedJSON := []byte(`{"mimeType":"mime-type","payload":"cGF5bG9hZA==", "annotations":{"a":"b","c":"d"}}`)
	// Don’t directly compare the JSON representation so that we don’t test for formatting differences, just verify that it contains exactly the expected data.
	var raw, expectedRaw map[string]any
	err = json.Unmarshal(res, &raw)
	require.NoError(t, err)
	err = json.Unmarshal(expectedJSON, &expectedRaw)
	require.NoError(t, err)
	assert.Equal(t, expectedRaw, raw)
}

func TestSigstoreUntrustedPayload(t *testing.T) {
	payload := []byte("payload")
	sig := SigstoreFromComponents("mime-type", payload,
		map[string]string{"a": "b", "c": "d"})
	assert.Equal(t, payload, sig.UntrustedPayload())
}

func TestSigstoreUntrustedAnnotations(t *testing.T) {
	annotations := map[string]string{"a": "b", "c": "d"}
	sig := SigstoreFromComponents("mime-type", []byte("payload"), annotations)
	assert.Equal(t, annotations, sig.UntrustedAnnotations())
}
