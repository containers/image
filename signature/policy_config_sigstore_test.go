package signature

import (
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xNewPRSigstoreSigned is like NewPRSigstoreSigned, except it must not fail.
func xNewPRSigstoreSigned(options ...PRSigstoreSignedOption) PolicyRequirement {
	pr, err := NewPRSigstoreSigned(options...)
	if err != nil {
		panic("xNewPRSigstoreSigned failed")
	}
	return pr
}

func TestNewPRSigstoreSigned(t *testing.T) {
	const testKeyPath = "/foo/bar"
	testKeyData := []byte("abc")
	testFulcio, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	const testRekorKeyPath = "/foo/baz"
	testRekorKeyData := []byte("def")
	testIdentity := NewPRMMatchRepoDigestOrExact()

	// Success: combinatoric combinations of key source and Rekor uses
	for _, c := range []struct {
		options       []PRSigstoreSignedOption
		requiresRekor bool
		expected      prSigstoreSigned
	}{
		{
			options: []PRSigstoreSignedOption{
				PRSigstoreSignedWithKeyPath(testKeyPath),
				PRSigstoreSignedWithSignedIdentity(testIdentity),
			},
			expected: prSigstoreSigned{
				prCommon:       prCommon{prTypeSigstoreSigned},
				KeyPath:        testKeyPath,
				KeyData:        nil,
				Fulcio:         nil,
				SignedIdentity: testIdentity,
			},
		},
		{
			options: []PRSigstoreSignedOption{
				PRSigstoreSignedWithKeyData(testKeyData),
				PRSigstoreSignedWithSignedIdentity(testIdentity),
			},
			expected: prSigstoreSigned{
				prCommon:       prCommon{prTypeSigstoreSigned},
				KeyPath:        "",
				KeyData:        testKeyData,
				Fulcio:         nil,
				SignedIdentity: testIdentity,
			},
		},
		{
			options: []PRSigstoreSignedOption{
				PRSigstoreSignedWithFulcio(testFulcio),
				PRSigstoreSignedWithSignedIdentity(testIdentity),
			},
			requiresRekor: true,
			expected: prSigstoreSigned{
				prCommon:       prCommon{prTypeSigstoreSigned},
				KeyPath:        "",
				KeyData:        nil,
				Fulcio:         testFulcio,
				SignedIdentity: testIdentity,
			},
		},
	} {
		for _, c2 := range []struct {
			rekorOptions  []PRSigstoreSignedOption
			rekorExpected prSigstoreSigned
		}{
			{ // No Rekor
				rekorOptions:  []PRSigstoreSignedOption{},
				rekorExpected: prSigstoreSigned{},
			},
			{
				rekorOptions: []PRSigstoreSignedOption{
					PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath),
				},
				rekorExpected: prSigstoreSigned{
					RekorPublicKeyPath: testRekorKeyPath,
				},
			},
			{
				rekorOptions: []PRSigstoreSignedOption{
					PRSigstoreSignedWithRekorPublicKeyData(testRekorKeyData),
				},
				rekorExpected: prSigstoreSigned{
					RekorPublicKeyData: testRekorKeyData,
				},
			},
		} {
			// Special-case this rejected combination:
			if c.requiresRekor && len(c2.rekorOptions) == 0 {
				continue
			}
			pr, err := newPRSigstoreSigned(append(c.options, c2.rekorOptions...)...)
			require.NoError(t, err)
			expected := c.expected // A shallow copy
			expected.RekorPublicKeyPath = c2.rekorExpected.RekorPublicKeyPath
			expected.RekorPublicKeyData = c2.rekorExpected.RekorPublicKeyData
			assert.Equal(t, &expected, pr)
		}
	}

	testFulcio2, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("test-user@example.com"),
	)
	require.NoError(t, err)
	for _, c := range [][]PRSigstoreSignedOption{
		{}, // None of keyPath nor keyData, fulcio specified
		{ // Both keyPath and keyData specified
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithKeyData(testKeyData),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // both keyPath and fulcio specified
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithFulcio(testFulcio),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // both keyData and fulcio specified
			PRSigstoreSignedWithKeyData(testKeyData),
			PRSigstoreSignedWithFulcio(testFulcio),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Duplicate keyPath
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithKeyPath(testKeyPath + "1"),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Duplicate keyData
			PRSigstoreSignedWithKeyData(testKeyData),
			PRSigstoreSignedWithKeyData([]byte("def")),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Duplicate fulcio
			PRSigstoreSignedWithFulcio(testFulcio),
			PRSigstoreSignedWithFulcio(testFulcio2),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // fulcio without Rekor
			PRSigstoreSignedWithFulcio(testFulcio),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Both rekorKeyPath and rekorKeyData specified
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath),
			PRSigstoreSignedWithRekorPublicKeyData(testRekorKeyData),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Duplicate rekorKeyPath
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath),
			PRSigstoreSignedWithRekorPublicKeyPath(testRekorKeyPath + "1"),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Duplicate keyData
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithRekorPublicKeyData(testRekorKeyData),
			PRSigstoreSignedWithRekorPublicKeyData([]byte("def")),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Missing signedIdentity
			PRSigstoreSignedWithKeyPath(testKeyPath),
		},
		{ // Duplicate signedIdentity}
			PRSigstoreSignedWithKeyPath(testKeyPath),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
			PRSigstoreSignedWithSignedIdentity(newPRMMatchRepository()),
		},
	} {
		_, err = newPRSigstoreSigned(c...)
		assert.Error(t, err)
	}
}

func TestNewPRSigstoreSignedKeyPath(t *testing.T) {
	const testPath = "/foo/bar"
	signedIdentity := NewPRMMatchRepoDigestOrExact()
	_pr, err := NewPRSigstoreSignedKeyPath(testPath, signedIdentity)
	require.NoError(t, err)
	pr, ok := _pr.(*prSigstoreSigned)
	require.True(t, ok)
	assert.Equal(t, &prSigstoreSigned{
		prCommon:       prCommon{Type: prTypeSigstoreSigned},
		KeyPath:        testPath,
		SignedIdentity: NewPRMMatchRepoDigestOrExact(),
	}, pr)
}

func TestNewPRSigstoreSignedKeyData(t *testing.T) {
	testData := []byte("abc")
	signedIdentity := NewPRMMatchRepoDigestOrExact()
	_pr, err := NewPRSigstoreSignedKeyData(testData, signedIdentity)
	require.NoError(t, err)
	pr, ok := _pr.(*prSigstoreSigned)
	require.True(t, ok)
	assert.Equal(t, &prSigstoreSigned{
		prCommon:       prCommon{Type: prTypeSigstoreSigned},
		KeyData:        testData,
		SignedIdentity: NewPRMMatchRepoDigestOrExact(),
	}, pr)
}

// Return the result of modifying validJSON with fn and unmarshaling it into *pr
func tryUnmarshalModifiedSigstoreSigned(t *testing.T, pr *prSigstoreSigned, validJSON []byte, modifyFn func(mSI)) error {
	var tmp mSI
	err := json.Unmarshal(validJSON, &tmp)
	require.NoError(t, err)

	modifyFn(tmp)

	*pr = prSigstoreSigned{}
	return jsonUnmarshalFromObject(t, tmp, &pr)
}

func TestPRSigstoreSignedUnmarshalJSON(t *testing.T) {
	keyDataTests := policyJSONUmarshallerTests{
		newDest: func() json.Unmarshaler { return &prSigstoreSigned{} },
		newValidObject: func() (interface{}, error) {
			return NewPRSigstoreSignedKeyData([]byte("abc"), NewPRMMatchRepoDigestOrExact())
		},
		otherJSONParser: func(validJSON []byte) (interface{}, error) {
			return newPolicyRequirementFromJSON(validJSON)
		},
		breakFns: []func(mSI){
			// The "type" field is missing
			func(v mSI) { delete(v, "type") },
			// Wrong "type" field
			func(v mSI) { v["type"] = 1 },
			func(v mSI) { v["type"] = "this is invalid" },
			// Extra top-level sub-object
			func(v mSI) { v["unexpected"] = 1 },
			// All of "keyPath" and "keyData", and "fulcio" is missing
			func(v mSI) { delete(v, "keyData") },
			// Both "keyPath" and "keyData" is present
			func(v mSI) { v["keyPath"] = "/foo/bar" },
			// Both "keyData" and "fulcio" is present
			func(v mSI) {
				v["fulcio"] = mSI{
					"caPath":       "/foo/baz",
					"oidcIssuer":   "https://example.com",
					"subjectEmail": "test@example.com",
				}
			},
			// Invalid "keyPath" field
			func(v mSI) { delete(v, "keyData"); v["keyPath"] = 1 },
			// Invalid "keyData" field
			func(v mSI) { v["keyData"] = 1 },
			func(v mSI) { v["keyData"] = "this is invalid base64" },
			// Invalid "fulcio" field
			func(v mSI) { v["fulcio"] = 1 },
			func(v mSI) { v["fulcio"] = mSI{} },
			// "fulcio" is explicit nil
			func(v mSI) { v["fulcio"] = nil },
			// Both "rekorKeyPath" and "rekorKeyData" is present
			func(v mSI) {
				v["rekorPublicKeyPath"] = "/foo/baz"
				v["rekorPublicKeyData"] = ""
			},
			// Invalid "rekorPublicKeyPath" field
			func(v mSI) { v["rekorPublicKeyPath"] = 1 },
			// Invalid "rekorPublicKeyData" field
			func(v mSI) { v["rekorPublicKeyData"] = 1 },
			func(v mSI) { v["rekorPublicKeyData"] = "this is invalid base64" },
			// Invalid "signedIdentity" field
			func(v mSI) { v["signedIdentity"] = "this is invalid" },
			// "signedIdentity" an explicit nil
			func(v mSI) { v["signedIdentity"] = nil },
		},
		duplicateFields: []string{"type", "keyData", "signedIdentity"},
	}
	keyDataTests.run(t)
	// Test keyPath-specific duplicate fields
	policyJSONUmarshallerTests{
		newDest: func() json.Unmarshaler { return &prSigstoreSigned{} },
		newValidObject: func() (interface{}, error) {
			return NewPRSigstoreSignedKeyPath("/foo/bar", NewPRMMatchRepoDigestOrExact())
		},
		otherJSONParser: func(validJSON []byte) (interface{}, error) {
			return newPolicyRequirementFromJSON(validJSON)
		},
		duplicateFields: []string{"type", "keyPath", "signedIdentity"},
	}.run(t)
	// Test Fulcio and rekorPublicKeyPath duplicate fields
	testFulcio, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	policyJSONUmarshallerTests{
		newDest: func() json.Unmarshaler { return &prSigstoreSigned{} },
		newValidObject: func() (interface{}, error) {
			return NewPRSigstoreSigned(
				PRSigstoreSignedWithFulcio(testFulcio),
				PRSigstoreSignedWithRekorPublicKeyPath("/foo/rekor"),
				PRSigstoreSignedWithSignedIdentity(NewPRMMatchRepoDigestOrExact()),
			)
		},
		otherJSONParser: func(validJSON []byte) (interface{}, error) {
			return newPolicyRequirementFromJSON(validJSON)
		},
		duplicateFields: []string{"type", "fulcio", "rekorPublicKeyPath", "signedIdentity"},
	}.run(t)
	// Test rekorPublicKeyData duplicate fields
	policyJSONUmarshallerTests{
		newDest: func() json.Unmarshaler { return &prSigstoreSigned{} },
		newValidObject: func() (interface{}, error) {
			return NewPRSigstoreSigned(
				PRSigstoreSignedWithKeyPath("/foo/bar"),
				PRSigstoreSignedWithRekorPublicKeyData([]byte("foo")),
				PRSigstoreSignedWithSignedIdentity(NewPRMMatchRepoDigestOrExact()),
			)
		},
		otherJSONParser: func(validJSON []byte) (interface{}, error) {
			return newPolicyRequirementFromJSON(validJSON)
		},
		duplicateFields: []string{"type", "keyPath", "rekorPublicKeyData", "signedIdentity"},
	}.run(t)

	var pr prSigstoreSigned

	// Start with a valid JSON.
	_, validJSON := keyDataTests.validObjectAndJSON(t)

	// Various allowed modifications to the requirement
	allowedModificationFns := []func(mSI){
		// Delete the signedIdentity field
		func(v mSI) { delete(v, "signedIdentity") },
	}
	for _, fn := range allowedModificationFns {
		err := tryUnmarshalModifiedSigstoreSigned(t, &pr, validJSON, fn)
		require.NoError(t, err)
	}

	// Various ways to set signedIdentity to the default value
	signedIdentityDefaultFns := []func(mSI){
		// Set signedIdentity to the default explicitly
		func(v mSI) { v["signedIdentity"] = NewPRMMatchRepoDigestOrExact() },
		// Delete the signedIdentity field
		func(v mSI) { delete(v, "signedIdentity") },
	}
	for _, fn := range signedIdentityDefaultFns {
		err := tryUnmarshalModifiedSigstoreSigned(t, &pr, validJSON, fn)
		require.NoError(t, err)
		assert.Equal(t, NewPRMMatchRepoDigestOrExact(), pr.SignedIdentity)
	}
}

func TestNewPRSigstoreSignedFulcio(t *testing.T) {
	const testCAPath = "/foo/bar"
	testCAData := []byte("abc")
	const testOIDCIssuer = "https://example.com"
	const testSubjectEmail = "test@example.com"

	// Success:
	for _, c := range []struct {
		options  []PRSigstoreSignedFulcioOption
		expected prSigstoreSignedFulcio
	}{
		{
			options: []PRSigstoreSignedFulcioOption{
				PRSigstoreSignedFulcioWithCAPath(testCAPath),
				PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
				PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
			},
			expected: prSigstoreSignedFulcio{
				CAPath:       testCAPath,
				OIDCIssuer:   testOIDCIssuer,
				SubjectEmail: testSubjectEmail,
			},
		},
		{
			options: []PRSigstoreSignedFulcioOption{
				PRSigstoreSignedFulcioWithCAData(testCAData),
				PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
				PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
			},
			expected: prSigstoreSignedFulcio{
				CAData:       testCAData,
				OIDCIssuer:   testOIDCIssuer,
				SubjectEmail: testSubjectEmail,
			},
		},
	} {
		pr, err := newPRSigstoreSignedFulcio(c.options...)
		require.NoError(t, err)
		assert.Equal(t, &c.expected, pr)
	}

	for _, c := range [][]PRSigstoreSignedFulcioOption{
		{ // Neither caPath nor caData specified
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{ // Both caPath and caData specified
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithCAData(testCAData),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{ // Duplicate caPath
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithCAPath(testCAPath + "1"),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{ // Duplicate caData
			PRSigstoreSignedFulcioWithCAData(testCAData),
			PRSigstoreSignedFulcioWithCAData([]byte("def")),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{ // Missing oidcIssuer
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{ // Duplicate oidcIssuer
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer + "1"),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
		},
		{ // Missing subjectEmail
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
		},
		{ // Duplicate subjectEmail
			PRSigstoreSignedFulcioWithCAPath(testCAPath),
			PRSigstoreSignedFulcioWithOIDCIssuer(testOIDCIssuer),
			PRSigstoreSignedFulcioWithSubjectEmail(testSubjectEmail),
			PRSigstoreSignedFulcioWithSubjectEmail("1" + testSubjectEmail),
		},
	} {
		_, err := newPRSigstoreSignedFulcio(c...)
		logrus.Errorf("%#v", err)
		assert.Error(t, err)
	}
}

func TestPRSigstoreSignedFulcioUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests{
		newDest: func() json.Unmarshaler { return &prSigstoreSignedFulcio{} },
		newValidObject: func() (interface{}, error) {
			return NewPRSigstoreSignedFulcio(
				PRSigstoreSignedFulcioWithCAPath("fixtures/fulcio_v1.crt.pem"),
				PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
				PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
			)
		},
		otherJSONParser: nil,
		breakFns: []func(mSI){
			// Extra top-level sub-object
			func(v mSI) { v["unexpected"] = 1 },
			// Both of "caPath" and "caData" are missing
			func(v mSI) { delete(v, "caPath") },
			// Both "caPath" and "caData" is present
			func(v mSI) { v["caData"] = "" },
			// Invalid "caPath" field
			func(v mSI) { v["caPath"] = 1 },
			// Invalid "oidcIssuer" field
			func(v mSI) { v["oidcIssuer"] = 1 },
			// "oidcIssuer" is missing
			func(v mSI) { delete(v, "oidcIssuer") },
			// Invalid "subjectEmail" field
			func(v mSI) { v["subjectEmail"] = 1 },
			// "subjectEmail" is missing
			func(v mSI) { delete(v, "subjectEmail") },
		},
		duplicateFields: []string{"caPath", "oidcIssuer", "subjectEmail"},
	}.run(t)
	// Test caData specifics
	policyJSONUmarshallerTests{
		newDest: func() json.Unmarshaler { return &prSigstoreSignedFulcio{} },
		newValidObject: func() (interface{}, error) {
			return NewPRSigstoreSignedFulcio(
				PRSigstoreSignedFulcioWithCAData([]byte("abc")),
				PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
				PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
			)
		},
		otherJSONParser: nil,
		breakFns: []func(mSI){
			// Invalid "caData" field
			func(v mSI) { v["caData"] = 1 },
			func(v mSI) { v["caData"] = "this is invalid base64" },
		},
		duplicateFields: []string{"caData", "oidcIssuer", "subjectEmail"},
	}.run(t)
}
