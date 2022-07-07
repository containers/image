package signature

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCosignFromComponents(t *testing.T) {
	const mimeType = "mime-type"
	payload := []byte("payload")
	annotations := map[string]string{"a": "b", "c": "d"}

	sig := CosignFromComponents(mimeType, payload, annotations)
	assert.Equal(t, Cosign{
		untrustedMIMEType:    mimeType,
		untrustedPayload:     payload,
		untrustedAnnotations: annotations,
	}, sig)
}

func TestCosignFromBlobChunk(t *testing.T) {
	// Success
	json := []byte(`{"mimeType":"mime-type","payload":"cGF5bG9hZA==", "annotations":{"a":"b","c":"d"}}`)
	res, err := cosignFromBlobChunk(json)
	require.NoError(t, err)
	assert.Equal(t, "mime-type", res.UntrustedMIMEType())
	assert.Equal(t, []byte("payload"), res.UntrustedPayload())
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, res.UntrustedAnnotations())

	// Invalid JSON
	_, err = cosignFromBlobChunk([]byte("&"))
	assert.Error(t, err)
}

func TestCosignFormatID(t *testing.T) {
	sig := CosignFromComponents("mime-type", []byte("payload"),
		map[string]string{"a": "b", "c": "d"})
	assert.Equal(t, CosignFormat, sig.FormatID())
}

func TestCosign_blobChunk(t *testing.T) {
	sig := CosignFromComponents("mime-type", []byte("payload"),
		map[string]string{"a": "b", "c": "d"})
	res, err := sig.blobChunk()
	require.NoError(t, err)

	expectedJSON := []byte(`{"mimeType":"mime-type","payload":"cGF5bG9hZA==", "annotations":{"a":"b","c":"d"}}`)
	// Don’t directly compare the JSON representation so that we don’t test for formatting differences, just verify that it contains exactly the expected data.
	var raw, expectedRaw map[string]interface{}
	err = json.Unmarshal(res, &raw)
	require.NoError(t, err)
	err = json.Unmarshal(expectedJSON, &expectedRaw)
	require.NoError(t, err)
	assert.Equal(t, expectedRaw, raw)
}

func TestCosign_UntrustedPayload(t *testing.T) {
	var payload = []byte("payload")
	sig := CosignFromComponents("mime-type", payload,
		map[string]string{"a": "b", "c": "d"})
	assert.Equal(t, payload, sig.UntrustedPayload())
}

func TestCosign_UntrustedAnnotations(t *testing.T) {
	annotations := map[string]string{"a": "b", "c": "d"}
	sig := CosignFromComponents("mime-type", []byte("payload"), annotations)
	assert.Equal(t, annotations, sig.UntrustedAnnotations())
}
