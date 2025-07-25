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
	// TestKeyFingerprintPrimaryWithSubkey is the primary key fingerprint of signature/fixtures/public-key-with-subkey.gpg.
	TestKeyFingerprintPrimaryWithSubkey = "B9F4CDB9FD8C475BFA340AC38D54A947EE396F36"
	// TestKeyFingerprintSubkeyWithSubkey is the subkey fingerprint of signature/fixtures/public-key-with-subkey.gpg.
	TestKeyFingerprintSubkeyWithSubkey = "57D1D95BBC53BA0EAFF0718CD75B3109B3BAAEAF"
	// TestKeyFingerprintPrimaryWithRevokedSubkey is the primary key fingerprint of signature/fixtures/public-key-with-revoked-subkey.gpg.
	TestKeyFingerprintPrimaryWithRevokedSubkey = "6D88C5A1993648A17B2BB0DC6B8499AF63388D63"
	// TestKeyFingerprintSubkeyWithRevokedSubkey is the subkey fingerprint of signature/fixtures/public-key-with-revoked-subkey.gpg.
	TestKeyFingerprintSubkeyWithRevokedSubkey = "4D33404E00B470050709F3233EF0F93A1F602997"
	// TestPassphrase is the passphrase for TestKeyFingerprintWithPassphrase.
	TestPassphrase = "WithPassphrase123"
)

var (
	// TestFingerprintListWithKey slice of multiple fingerprints including the fingerprint of the private key in this directory.
	TestFingerprintListWithKey = []string{TestKeyFingerprint, TestOtherFingerprint1, TestOtherFingerprint2}
	// TestFingerprintListWithoutKey slice of multiple fingerprints not including the fingerprint of the private key in this directory.
	TestFingerprintListWithoutKey = []string{TestOtherFingerprint1, TestOtherFingerprint2}
)
