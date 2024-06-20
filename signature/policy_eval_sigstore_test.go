//go:build !containers_image_fulcio_stub
// +build !containers_image_fulcio_stub

// Policy evaluation for prCosignSigned.

package signature

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/containers/image/v5/internal/signature"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRSigstoreSignedFulcioPrepareTrustRoot(t *testing.T) {
	const testCAPath = "fixtures/fulcio_v1.crt.pem"
	testCAData, err := os.ReadFile(testCAPath)
	require.NoError(t, err)
	const testOIDCIssuer = "https://example.com"
	testSubjectEmail := "test@example.com"

	// Success
	for _, c := range [][]PRSigstoreSignedFulcioOption{
		{
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{
			PRSigstoreSignedFulcioWithCAData(testCAData),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
	} {
		f, err := newPRSigstoreSignedFulcio(c...)
		require.NoError(t, err)
		res, err := f.prepareTrustRoot()
		require.NoError(t, err)
		assert.NotNil(t, res.caCertificates) // Doing a better test seems hard; we would need to compare .Subjects with a DER encoding.
		assert.Equal(t, testOIDCIssuer, res.oidcIssuer)
		assert.Equal(t, testSubjectEmail, res.subjectEmail)
	}

	// Failure
	for _, f := range []prSigstoreSignedFulcio{ // Use a prSigstoreSignedFulcio because these configurations should be rejected by NewPRSigstoreSignedFulcio.
		{ // Neither CAPath nor CAData specified
			OIDCIssuer:   testOIDCIssuer,
			SubjectEmail: testSubjectEmail,
		},
		{ // Both CAPath and CAData specified
			CAPath:       testCAPath,
			CAData:       testCAData,
			OIDCIssuer:   testOIDCIssuer,
			SubjectEmail: testSubjectEmail,
		},
		{ // Invalid CAPath
			CAPath:       "fixtures/image.signature",
			OIDCIssuer:   testOIDCIssuer,
			SubjectEmail: testSubjectEmail,
		},
		{ // Unusable CAPath
			CAPath:       "fixtures/this/does/not/exist",
			OIDCIssuer:   testOIDCIssuer,
			SubjectEmail: testSubjectEmail,
		},
		{ // Invalid CAData
			CAData:       []byte("invalid"),
			OIDCIssuer:   testOIDCIssuer,
			SubjectEmail: testSubjectEmail,
		},
		{ // Missing OIDCIssuer
			CAPath:       testCAPath,
			SubjectEmail: testSubjectEmail,
		},
		{ // Missing SubjectEmail
			CAPath:     testCAPath,
			OIDCIssuer: testOIDCIssuer,
		},
	} {
		_, err := f.prepareTrustRoot()
		assert.Error(t, err)
	}
}

func TestPRSigstoreSignedPrepareTrustRoot(t *testing.T) {
	const testKeyPath = "fixtures/cosign.pub"
	const testKeyPath2 = "fixtures/cosign.pub"

	testKeyData, err := os.ReadFile(testKeyPath)
	require.NoError(t, err)
	testKeyData2, err := os.ReadFile(testKeyPath2)
	require.NoError(t, err)

	testFulcio, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	const testRekorPublicKeyPath = "fixtures/rekor.pub"
	testRekorPublicKeyData, err := os.ReadFile(testRekorPublicKeyPath)
	require.NoError(t, err)
	testIdentity := newPRMMatchRepoDigestOrExact()
	testIdentityOption := PRSigstoreSignedWithSignedIdentity(testIdentity)

	// Success with public key
	for _, c := range [][]PRSigstoreSignedOption{
		{
			PRSigstoreSignedWithKeyPath(testKeyPath),
			testIdentityOption,
		},
		{
			PRSigstoreSignedWithKeyData(testKeyData),
			testIdentityOption,
		},
		{
			PRSigstoreSignedWithKeyPaths([]string{testKeyPath, testKeyPath2}),
			testIdentityOption,
		},
		{
			PRSigstoreSignedWithKeyDatas([][]byte{testKeyData, testKeyData2}),
			testIdentityOption,
		},
	} {
		pr, err := newPRSigstoreSigned(c...)
		require.NoError(t, err)
		res, err := pr.prepareTrustRoot()
		require.NoError(t, err)
		assert.NotNil(t, res.publicKeys)
		//assert.Len(t, res.publicKeys, 1)
		assert.Nil(t, res.fulcio)
		assert.Nil(t, res.rekorPublicKey)
	}
	// Success with Fulcio
	pr, err := newPRSigstoreSigned(
		PRSigstoreSignedWithFulcio(testFulcio),
		PRSigstoreSignedWithRekorPublicKeyData(testRekorPublicKeyData),
		testIdentityOption,
	)
	require.NoError(t, err)
	res, err := pr.prepareTrustRoot()
	require.NoError(t, err)
	assert.Len(t, res.publicKeys, 0)
	assert.NotNil(t, res.fulcio)
	assert.NotNil(t, res.rekorPublicKey)
	// Success with Rekor public key
	for _, c := range [][]PRSigstoreSignedOption{
		{
			PRSigstoreSignedWithKeyData(testKeyData),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorPublicKeyPath),
			testIdentityOption,
		},
		{
			PRSigstoreSignedWithKeyData(testKeyData),
			PRSigstoreSignedWithRekorPublicKeyData(testRekorPublicKeyData),
			testIdentityOption,
		},
		{
			PRSigstoreSignedWithKeyPaths([]string{testKeyPath, testKeyPath2}),
			PRSigstoreSignedWithRekorPublicKeyData(testRekorPublicKeyData),
			testIdentityOption,
		},
		{
			PRSigstoreSignedWithKeyDatas([][]byte{testKeyData, testKeyData2}),
			PRSigstoreSignedWithRekorPublicKeyData(testRekorPublicKeyData),
			testIdentityOption,
		},
	} {
		pr, err := newPRSigstoreSigned(c...)
		require.NoError(t, err)
		res, err := pr.prepareTrustRoot()
		require.NoError(t, err)
		assert.NotNil(t, res.publicKeys)
		assert.Nil(t, res.fulcio)
		assert.NotNil(t, res.rekorPublicKey)
	}

	// Failure
	for _, pr := range []prSigstoreSigned{ // Use a prSigstoreSigned because these configurations should be rejected by NewPRSigstoreSigned.
		{ // Both KeyPath and KeyData specified
			KeyPath:        testKeyPath,
			KeyData:        testKeyData,
			SignedIdentity: testIdentity,
		},
		{ // Invalid public key path
			KeyPath:        "fixtures/image.signature",
			SignedIdentity: testIdentity,
		},
		{ // Unusable public key path
			KeyPath:        "fixtures/this/does/not/exist",
			SignedIdentity: testIdentity,
		},
		{ // Invalid public key data
			KeyData:        []byte("this is invalid"),
			SignedIdentity: testIdentity,
		},
		{ // Invalid Fulcio configuration
			Fulcio:             &prSigstoreSignedFulcio{},
			RekorPublicKeyData: testKeyData,
			SignedIdentity:     testIdentity,
		},
		{ // Both RekorPublicKeyPath and RekorPublicKeyData specified
			KeyData:            testKeyData,
			RekorPublicKeyPath: testRekorPublicKeyPath,
			RekorPublicKeyData: testRekorPublicKeyData,
			SignedIdentity:     testIdentity,
		},
		{ // Invalid Rekor public key path
			KeyData:            testKeyData,
			RekorPublicKeyPath: "fixtures/image.signature",
			SignedIdentity:     testIdentity,
		},
		{ // Invalid Rekor public key data
			KeyData:            testKeyData,
			RekorPublicKeyData: []byte("this is invalid"),
			SignedIdentity:     testIdentity,
		},
		{ // Rekor public key is not ECDSA
			KeyData:            testKeyData,
			RekorPublicKeyPath: "fixtures/some-rsa-key.pub",
			SignedIdentity:     testIdentity,
		},
	} {
		_, err = pr.prepareTrustRoot()
		assert.Error(t, err)
	}
}

func TestPRSigstoreSignedIsSignatureAuthorAccepted(t *testing.T) {
	// Currently, this fails even with a correctly signed image.
	prm := NewPRMMatchRepository() // We prefer to test with a Cosign-created signature for interoperability, and that doesn’t work with matchExact.
	testImage := dirImageMock(t, "fixtures/dir-img-cosign-valid", "192.168.64.2:5000/cosign-signed-single-sample")
	testImageSigBlob, err := os.ReadFile("fixtures/dir-img-cosign-valid/signature-1")
	require.NoError(t, err)

	// Successful validation, with KeyData and KeyPath
	pr, err := newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), testImage, testImageSigBlob)
	assertSARRejected(t, sar, parsedSig, err)
}

// sigstoreSignatureFromFile returns a signature.Sigstore loaded from path.
func sigstoreSignatureFromFile(t *testing.T, path string) signature.Sigstore {
	blob, err := os.ReadFile(path)
	require.NoError(t, err)
	genericSig, err := signature.FromBlob(blob)
	require.NoError(t, err)
	sig, ok := genericSig.(signature.Sigstore)
	require.True(t, ok)
	return sig
}

// sigstoreSignatureWithoutAnnotation returns a signature.Sigstore based on template
// that is missing the specified annotation.
func sigstoreSignatureWithoutAnnotation(t *testing.T, template signature.Sigstore, annotation string) signature.Sigstore {
	annotations := template.UntrustedAnnotations() // This returns a copy that is safe to modify.
	require.Contains(t, annotations, annotation)
	delete(annotations, annotation)
	return signature.SigstoreFromComponents(template.UntrustedMIMEType(), template.UntrustedPayload(), annotations)
}

// sigstoreSignatureWithModifiedAnnotation returns a signature.Sigstore based on template
// where the specified annotation is replaced
func sigstoreSignatureWithModifiedAnnotation(template signature.Sigstore, annotation, value string) signature.Sigstore {
	annotations := template.UntrustedAnnotations() // This returns a copy that is safe to modify.
	annotations[annotation] = value
	return signature.SigstoreFromComponents(template.UntrustedMIMEType(), template.UntrustedPayload(), annotations)
}

func TestPRrSigstoreSignedIsSignatureAccepted(t *testing.T) {
	assertAccepted := func(sar signatureAcceptanceResult, err error) {
		assert.Equal(t, sarAccepted, sar)
		assert.NoError(t, err)
	}
	assertRejected := func(sar signatureAcceptanceResult, err error) {
		logrus.Errorf("%v", err)
		assert.Equal(t, sarRejected, sar)
		assert.Error(t, err)
	}

	prm := NewPRMMatchRepository() // We prefer to test with a Cosign-created signature to ensure interoperability, and that doesn’t work with matchExact. matchExact is tested later.
	testKeyImage := dirImageMock(t, "fixtures/dir-img-cosign-valid", "192.168.64.2:5000/cosign-signed-single-sample")
	testKeyImageSig := sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-valid/signature-1")
	testKeyRekorImage := dirImageMock(t, "fixtures/dir-img-cosign-key-rekor-valid", "192.168.64.2:5000/cosign-signed/key-1")
	testKeyRekorImageSig := sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-key-rekor-valid/signature-1")
	testFulcioRekorImage := dirImageMock(t, "fixtures/dir-img-cosign-fulcio-rekor-valid", "192.168.64.2:5000/cosign-signed/fulcio-rekor-1")
	testFulcioRekorImageSig := sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-fulcio-rekor-valid/signature-1")
	keyData, err := os.ReadFile("fixtures/cosign.pub")
	require.NoError(t, err)

	// prepareTrustRoot fails
	pr := &prSigstoreSigned{
		KeyPath:        "fixtures/cosign.pub",
		KeyData:        keyData,
		SignedIdentity: prm,
	}
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err := pr.isSignatureAccepted(context.Background(), nil, testKeyImageSig)
	assertRejected(sar, err)

	// Signature has no cryptographic signature
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		signature.SigstoreFromComponents(testKeyImageSig.UntrustedMIMEType(), testKeyImageSig.UntrustedPayload(), nil))
	assertRejected(sar, err)

	// Neither a public key nor Fulcio is specified
	pr = &prSigstoreSigned{
		SignedIdentity: prm,
	}
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil, testKeyImageSig)
	assertRejected(sar, err)

	// Both a public key and Fulcio is specified
	fulcio, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	pr = &prSigstoreSigned{
		KeyPath:        "fixtures/cosign.pub",
		Fulcio:         fulcio,
		SignedIdentity: prm,
	}
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil, testKeyImageSig)
	assertRejected(sar, err)

	// Successful key+Rekor use
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign2.pub"),
		PRSigstoreSignedWithRekorPublicKeyPath("fixtures/rekor.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), testKeyRekorImage, testKeyRekorImageSig)
	require.NoError(t, err)
	assertAccepted(sar, err)

	// key+Rekor, missing Rekor SET annotation
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithoutAnnotation(t, testKeyRekorImageSig, signature.SigstoreSETAnnotationKey))
	assertRejected(sar, err)
	// Actual Rekor logic is unit-tested elsewhere, but smoke-test the basics:
	// key+Rekor: Invalid Rekor SET
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithModifiedAnnotation(testKeyRekorImageSig, signature.SigstoreSETAnnotationKey,
			"this is not a valid SET"))
	assertRejected(sar, err)
	// Fulcio: A Rekor SET which we don’t accept (one of many reasons)
	pr2, err := newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign2.pub"),
		PRSigstoreSignedWithRekorPublicKeyPath("fixtures/cosign.pub"), // not rekor.pub = a key mismatch
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr2.isSignatureAccepted(context.Background(), nil, testKeyRekorImageSig)
	assertRejected(sar, err)

	// Successful Fulcio certificate use
	fulcio, err = NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithFulcio(fulcio),
		PRSigstoreSignedWithRekorPublicKeyPath("fixtures/rekor.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), testFulcioRekorImage,
		testFulcioRekorImageSig)
	require.NoError(t, err)
	assertAccepted(sar, err)

	// Fulcio, no Rekor requirement
	pr2 = &prSigstoreSigned{
		Fulcio:         fulcio,
		SignedIdentity: prm,
	}
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr2.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithoutAnnotation(t, testFulcioRekorImageSig, signature.SigstoreSETAnnotationKey))
	assertRejected(sar, err)
	// Fulcio, missing Rekor SET annotation
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithoutAnnotation(t, testFulcioRekorImageSig, signature.SigstoreSETAnnotationKey))
	assertRejected(sar, err)
	// Fulcio, missing certificate annotation
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithoutAnnotation(t, testFulcioRekorImageSig, signature.SigstoreCertificateAnnotationKey))
	assertRejected(sar, err)
	// Fulcio: missing certificate chain annotation causes the Cosign-issued signature to be rejected
	// because there is no path to the trusted CA
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithoutAnnotation(t, testFulcioRekorImageSig, signature.SigstoreIntermediateCertificateChainAnnotationKey))
	assertRejected(sar, err)
	// … but a signature without the intermediate annotation is fine if the issuer is directly trusted
	// (which we handle by trusing the intermediates)
	fulcio2, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAData([]byte(testFulcioRekorImageSig.UntrustedAnnotations()[signature.SigstoreIntermediateCertificateChainAnnotationKey])),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	pr2, err = newPRSigstoreSigned(
		PRSigstoreSignedWithFulcio(fulcio2),
		PRSigstoreSignedWithRekorPublicKeyPath("fixtures/rekor.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr2.isSignatureAccepted(context.Background(), testFulcioRekorImage,
		sigstoreSignatureWithoutAnnotation(t, testFulcioRekorImageSig, signature.SigstoreIntermediateCertificateChainAnnotationKey))
	assertAccepted(sar, err)
	// Actual Fulcio and Rekor logic is unit-tested elsewhere, but smoke-test the basics:
	// Fulcio: Invalid Fulcio certificate
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithModifiedAnnotation(testFulcioRekorImageSig, signature.SigstoreCertificateAnnotationKey,
			"this is not a valid certificate"))
	assertRejected(sar, err)
	// Fulcio: A Fulcio certificate which we don’t accept (one of many reasons)
	fulcio2, err = NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("this-does-not-match@example.com"),
	)
	require.NoError(t, err)
	pr2, err = newPRSigstoreSigned(
		PRSigstoreSignedWithFulcio(fulcio2),
		PRSigstoreSignedWithRekorPublicKeyPath("fixtures/rekor.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr2.isSignatureAccepted(context.Background(), nil, testFulcioRekorImageSig)
	assertRejected(sar, err)
	// Fulcio: Invalid Rekor SET
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		sigstoreSignatureWithModifiedAnnotation(testFulcioRekorImageSig, signature.SigstoreSETAnnotationKey,
			"this is not a valid SET"))
	assertRejected(sar, err)
	// Fulcio: A Rekor SET which we don’t accept (one of many reasons)
	pr2, err = newPRSigstoreSigned(
		PRSigstoreSignedWithFulcio(fulcio),
		PRSigstoreSignedWithRekorPublicKeyPath("fixtures/cosign.pub"), // not rekor.pub = a key mismatch
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr2.isSignatureAccepted(context.Background(), nil, testFulcioRekorImageSig)
	assertRejected(sar, err)

	// Successful validation, with KeyData and KeyPath
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), testKeyImage, testKeyImageSig)
	assertAccepted(sar, err)

	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyData(keyData),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), testKeyImage, testKeyImageSig)
	assertAccepted(sar, err)

	// A signature which does not verify
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil,
		signature.SigstoreFromComponents(testKeyImageSig.UntrustedMIMEType(), testKeyImageSig.UntrustedPayload(), map[string]string{
			signature.SigstoreSignatureAnnotationKey: base64.StdEncoding.EncodeToString([]byte("invalid signature")),
		}))
	assertRejected(sar, err)

	// A valid signature using an unknown key.
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	sar, err = pr.isSignatureAccepted(context.Background(), nil, sigstoreSignatureFromFile(t, "fixtures/unknown-cosign-key.signature"))
	assertRejected(sar, err)

	// A valid signature with a rejected identity.
	nonmatchingPRM, err := NewPRMExactReference("this/does-not:match")
	require.NoError(t, err)
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(nonmatchingPRM),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), testKeyImage, testKeyImageSig)
	assertRejected(sar, err)

	// Error reading image manifest
	image := dirImageMock(t, "fixtures/dir-img-cosign-no-manifest", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), image, sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-no-manifest/signature-1"))
	assertRejected(sar, err)

	// Error computing manifest digest
	image = dirImageMock(t, "fixtures/dir-img-cosign-manifest-digest-error", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), image, sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-manifest-digest-error/signature-1"))
	assertRejected(sar, err)

	// A valid signature with a non-matching manifest
	image = dirImageMock(t, "fixtures/dir-img-cosign-modified-manifest", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), image, sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-modified-manifest/signature-1"))
	assertRejected(sar, err)

	// Minimally check that the prmMatchExact also works as expected:
	// - Signatures with a matching tag work
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid-with-tag", "192.168.64.2:5000/skopeo-signed:tag")
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(NewPRMMatchExact()),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), image, sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-valid-with-tag/signature-1"))
	assertAccepted(sar, err)
	// - Signatures with a non-matching tag are rejected
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid-with-tag", "192.168.64.2:5000/skopeo-signed:othertag")
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(NewPRMMatchExact()),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), image, sigstoreSignatureFromFile(t, "fixtures/dir-img-cosign-valid-with-tag/signature-1"))
	assertRejected(sar, err)
	// - Cosign-created signatures are rejected
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(NewPRMMatchExact()),
	)
	require.NoError(t, err)
	sar, err = pr.isSignatureAccepted(context.Background(), testKeyImage, testKeyImageSig)
	assertRejected(sar, err)
}

func TestPRSigstoreSignedIsRunningImageAllowed(t *testing.T) {
	prm := NewPRMMatchRepository() // We prefer to test with a Cosign-created signature to ensure interoperability, and that doesn’t work with matchExact. matchExact is tested later.

	// A simple success case: single valid signature.
	image := dirImageMock(t, "fixtures/dir-img-cosign-valid", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err := NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err := pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)

	// Error reading signatures
	invalidSigDir := createInvalidSigDir(t)
	image = dirImageMock(t, invalidSigDir, "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejected(t, allowed, err)

	// No signatures
	image = dirImageMock(t, "fixtures/dir-img-unsigned", "testing/manifest:latest")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejected(t, allowed, err)

	// Only non-sigstore signatures
	image = dirImageMock(t, "fixtures/dir-img-valid", "testing/manifest:latest")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejected(t, allowed, err)

	// Only non-signature sigstore attachments
	image = dirImageMock(t, "fixtures/dir-img-cosign-other-attachment", "testing/manifest:latest")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejected(t, allowed, err)

	// 1 invalid signature: use dir-img-valid, but a non-matching Docker reference
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid", "testing/manifest:notlatest")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)

	// 2 valid signatures
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid-2", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)

	// One invalid, one valid signature (in this order)
	image = dirImageMock(t, "fixtures/dir-img-cosign-mixed", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)

	// 2 invalid signajtures: use dir-img-cosign-valid-2, but a non-matching Docker reference
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid-2", "this/does-not:match")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(prm),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)

	// Minimally check that the prmMatchExact also works as expected:
	// - Signatures with a matching tag work
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid-with-tag", "192.168.64.2:5000/skopeo-signed:tag")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(NewPRMMatchExact()),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)
	// - Signatures with a non-matching tag are rejected
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid-with-tag", "192.168.64.2:5000/skopeo-signed:othertag")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(NewPRMMatchExact()),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)
	// - Cosign-created signatures are rejected
	image = dirImageMock(t, "fixtures/dir-img-cosign-valid", "192.168.64.2:5000/cosign-signed-single-sample")
	pr, err = NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath("fixtures/cosign.pub"),
		PRSigstoreSignedWithSignedIdentity(NewPRMMatchExact()),
	)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)
}
