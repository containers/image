package copy

import (
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/containers/ocicrypt"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// isOciEncrypted returns a bool indicating if a mediatype is encrypted
// This function will be moved to be part of OCI spec when adopted.
func isOciEncrypted(mediatype string) bool {
	return strings.HasSuffix(mediatype, "+encrypted")
}

// isEncrypted checks if an image is encrypted
func isEncrypted(i types.Image) bool {
	layers := i.LayerInfos()
	for _, l := range layers {
		if isOciEncrypted(l.MediaType) {
			return true
		}
	}
	return false
}

// bpDecryptionStepData contains data that the copy pipeline needs about the decryption step.
type bpDecryptionStepData struct {
	decrypting bool // We are actually decrypting the stream
}

// blobPipelineDecryptionStep updates *stream to decrypt if, it necessary.
// srcInfo is only used for error messages.
// Returns data for other steps; the caller should eventually use updateCryptoOperation.
func (c *copier) blobPipelineDecryptionStep(stream *sourceStream, srcInfo types.BlobInfo) (*bpDecryptionStepData, error) {
	var decrypted bool
	if isOciEncrypted(stream.info.MediaType) && c.ociDecryptConfig != nil {
		newDesc := imgspecv1.Descriptor{
			Annotations: stream.info.Annotations,
		}

		var d digest.Digest
		reader, d, err := ocicrypt.DecryptLayer(c.ociDecryptConfig, stream.reader, newDesc, false)
		if err != nil {
			return nil, errors.Wrapf(err, "decrypting layer %s", srcInfo.Digest)
		}
		stream.reader = reader

		stream.info.Digest = d
		stream.info.Size = -1
		for k := range stream.info.Annotations {
			if strings.HasPrefix(k, "org.opencontainers.image.enc") {
				delete(stream.info.Annotations, k)
			}
		}
		decrypted = true
	}
	return &bpDecryptionStepData{
		decrypting: decrypted,
	}, nil
}

// updateCryptoOperation sets *operation, if necessary.
func (d *bpDecryptionStepData) updateCryptoOperation(operation *types.LayerCrypto) {
	if d.decrypting {
		*operation = types.Decrypt
	}
}
