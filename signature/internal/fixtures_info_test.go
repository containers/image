package internal

import "github.com/opencontainers/go-digest"

const (
	// TestImageManifestDigest is the Docker manifest digest of "image.manifest.json"
	TestImageManifestDigest = digest.Digest("sha256:20bf21ed457b390829cdbeec8795a7bea1626991fda603e0d01b4e7f60427e55")
	// TestImageSignatureReference is the Docker image reference signed in "image.signature"
	TestImageSignatureReference = "testing/manifest"

	// TestCosignManifestDigest is the manifest digest of "valid.signature"
	TestCosignManifestDigest = digest.Digest("sha256:634a8f35b5f16dcf4aaa0822adc0b1964bb786fca12f6831de8ddc45e5986a00")
	// TestCosignSignatureReference is the Docker reference signed in "valid.signature"
	TestCosignSignatureReference = "192.168.64.2:5000/cosign-signed-single-sample"
)
