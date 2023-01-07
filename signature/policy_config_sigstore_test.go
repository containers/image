package signature

import (
	"encoding/json"
	"testing"

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
	const testPath = "/foo/bar"
	testData := []byte("abc")
	testIdentity := NewPRMMatchRepoDigestOrExact()

	// Success
	pr, err := newPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath(testPath),
		PRSigstoreSignedWithSignedIdentity(testIdentity),
	)
	require.NoError(t, err)
	assert.Equal(t, &prSigstoreSigned{
		prCommon:       prCommon{prTypeSigstoreSigned},
		KeyPath:        testPath,
		KeyData:        nil,
		SignedIdentity: testIdentity,
	}, pr)
	pr, err = newPRSigstoreSigned(
		PRSigstoreSignedWithKeyData(testData),
		PRSigstoreSignedWithSignedIdentity(testIdentity),
	)
	require.NoError(t, err)
	assert.Equal(t, &prSigstoreSigned{
		prCommon:       prCommon{prTypeSigstoreSigned},
		KeyPath:        "",
		KeyData:        testData,
		SignedIdentity: testIdentity,
	}, pr)

	for _, c := range [][]PRSigstoreSignedOption{
		{ // Both keyPath and keyData specified
			PRSigstoreSignedWithKeyPath(testPath),
			PRSigstoreSignedWithKeyData(testData),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{}, // Neither keyPath nor keyData specified
		{ // Duplicate keyPath
			PRSigstoreSignedWithKeyPath(testPath),
			PRSigstoreSignedWithKeyPath(testPath + "1"),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Duplicate keyData
			PRSigstoreSignedWithKeyData(testData),
			PRSigstoreSignedWithKeyData([]byte("def")),
			PRSigstoreSignedWithSignedIdentity(testIdentity),
		},
		{ // Missing signedIdentity
			PRSigstoreSignedWithKeyPath(testPath),
		},
		{ // Duplicate signedIdentity}
			PRSigstoreSignedWithKeyPath(testPath),
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
			// Both "keyPath" and "keyData" is missing
			func(v mSI) { delete(v, "keyData") },
			// Both "keyPath" and "keyData" is present
			func(v mSI) { v["keyPath"] = "/foo/bar" },
			// Invalid "keyPath" field
			func(v mSI) { delete(v, "keyData"); v["keyPath"] = 1 },
			func(v mSI) { v["type"] = "this is invalid" },
			// Invalid "keyData" field
			func(v mSI) { v["keyData"] = 1 },
			func(v mSI) { v["keyData"] = "this is invalid base64" },
			// Invalid "signedIdentity" field
			func(v mSI) { v["signedIdentity"] = "this is invalid" },
			// "signedIdentity" an explicit nil
			func(v mSI) { v["signedIdentity"] = nil },
		},
		duplicateFields: []string{"type", "keyData", "signedIdentity"},
	}
	keyDataTests.run(t)
	// Test the keyPath-specific aspects
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
