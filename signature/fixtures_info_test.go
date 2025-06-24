package signature

import "github.com/opencontainers/go-digest"

const (
	// TestImageManifestDigest is the Docker manifest digest of "image.manifest.json"
	TestImageManifestDigest = digest.Digest("sha256:20bf21ed457b390829cdbeec8795a7bea1626991fda603e0d01b4e7f60427e55")
	// TestImageSignatureReference is the Docker image reference signed in "image.signature"
	TestImageSignatureReference = "testing/manifest"
	// TestKeyFingerprint is the fingerprint of the private key in this directory.
	TestKeyFingerprint = "08CD26E446E2E95249B7A405E932F44B23E8DD43"
	// TestOtherFingerprint1 is a random fingerprint.
	TestOtherFingerprint1 = "0123456789ABCDEF0123456789ABCDEF01234567"
	// TestOtherFingerprint2 is a random fingerprint.
	TestOtherFingerprint2 = "DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF"
	// TestKeyShortID is the short ID of the private key in this directory.
	TestKeyShortID = "E932F44B23E8DD43"
	// TestKeyFingerprintWithPassphrase is the fingerprint of the private key with passphrase in this directory.
	TestKeyFingerprintWithPassphrase = "F2B501009F78B0B340221A12A3CD242DA6028093"
	// TestPassphrase is the passphrase for TestKeyFingerprintWithPassphrase.
	TestPassphrase = "WithPassphrase123"
)

var (
	// TestFingerprintListWithKey slice of multiple fingerprints including the fingerprint of the private key in this directory.
	TestFingerprintListWithKey = []string{TestKeyFingerprint, TestOtherFingerprint1, TestOtherFingerprint2}
	// TestFingerprintListWithoutKey slice of multiple fingerprints not including the fingerprint of the private key in this directory.
	TestFingerprintListWithoutKey = []string{TestOtherFingerprint1, TestOtherFingerprint2}
)
