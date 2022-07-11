package signature

import "encoding/json"

const (
	// from sigstore/cosign/pkg/types.SimpleSigningMediaType
	CosignSignatureMIMEType = "application/vnd.dev.cosign.simplesigning.v1+json"
	// from sigstore/cosign/pkg/oci/static.SignatureAnnotationKey
	CosignSignatureAnnotationKey = "dev.cosignproject.cosign/signature"
)

// Cosign is a github.com/Cosign/cosign signature.
// For the persistent-storage format used for blobChunk(), we want
// a degree of forward compatibility against unexpected field changes
// (as has happened before), which is why this data type
// contains just a payload + annotations (including annotations
// that we don’t recognize or support), instead of individual fields
// for the known annotations.
type Cosign struct {
	untrustedMIMEType    string
	untrustedPayload     []byte
	untrustedAnnotations map[string]string
}

// cosignJSONRepresentation needs the files to be public, which we don’t want for
// the main Cosign type.
type cosignJSONRepresentation struct {
	UntrustedMIMEType    string            `json:"mimeType"`
	UntrustedPayload     []byte            `json:"payload"`
	UntrustedAnnotations map[string]string `json:"annotations"`
}

// CosignFromComponents returns a Cosign object from its components.
func CosignFromComponents(untrustedMimeType string, untrustedPayload []byte, untrustedAnnotations map[string]string) Cosign {
	return Cosign{
		untrustedMIMEType:    untrustedMimeType,
		untrustedPayload:     copyByteSlice(untrustedPayload),
		untrustedAnnotations: copyStringMap(untrustedAnnotations),
	}
}

// cosignFromBlobChunk converts a Cosign signature, as returned by Cosign.blobChunk, into a Cosign object.
func cosignFromBlobChunk(blobChunk []byte) (Cosign, error) {
	var v cosignJSONRepresentation
	if err := json.Unmarshal(blobChunk, &v); err != nil {
		return Cosign{}, err
	}
	return CosignFromComponents(v.UntrustedMIMEType,
		v.UntrustedPayload,
		v.UntrustedAnnotations), nil
}

func (s Cosign) FormatID() FormatID {
	return CosignFormat
}

// blobChunk returns a representation of signature as a []byte, suitable for long-term storage.
// Almost everyone should use signature.Blob() instead.
func (s Cosign) blobChunk() ([]byte, error) {
	return json.Marshal(cosignJSONRepresentation{
		UntrustedMIMEType:    s.UntrustedMIMEType(),
		UntrustedPayload:     s.UntrustedPayload(),
		UntrustedAnnotations: s.UntrustedAnnotations(),
	})
}

func (s Cosign) UntrustedMIMEType() string {
	return s.untrustedMIMEType
}
func (s Cosign) UntrustedPayload() []byte {
	return copyByteSlice(s.untrustedPayload)
}

func (s Cosign) UntrustedAnnotations() map[string]string {
	return copyStringMap(s.untrustedAnnotations)
}

func copyStringMap(m map[string]string) map[string]string {
	res := map[string]string{}
	for k, v := range m {
		res[k] = v
	}
	return res
}
