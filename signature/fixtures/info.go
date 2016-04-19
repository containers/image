package fixtures

const (
	// TestImageManifestDigest is the Docker manifest digest of "image.manifest.json"
	TestImageManifestDigest = "sha256:20bf21ed457b390829cdbeec8795a7bea1626991fda603e0d01b4e7f60427e55"
	// TestV1S1ManifestDigest is the Docker manifest digest of "v1s1.manifest.json"
	TestV1S1ManifestDigest = "sha256:077594da70fc17ec2c93cfa4e6ed1fcc26992851fb2c71861338aaf4aa9e41b1"
	// TestImageSignatureReference is the Docker image reference signed in "image.signature"
	TestImageSignatureReference = "testing/manifest"
	// TestKeyFingerprint is the fingerprint of the private key in this directory.
	TestKeyFingerprint = "1D8230F6CDB6A06716E414C1DB72F2188BB46CC8"
)
