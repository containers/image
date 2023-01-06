package signature

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xNewPRSigstoreSignedKeyPath is like NewPRSigstoreSignedKeyPath, except it must not fail.
func xNewPRSigstoreSignedKeyPath(keyPath string, signedIdentity PolicyReferenceMatch) PolicyRequirement {
	pr, err := NewPRSigstoreSignedKeyPath(keyPath, signedIdentity)
	if err != nil {
		panic("xNewPRSigstoreSignedKeyPath failed")
	}
	return pr
}

// xNewPRSigstoreSignedKeyData is like NewPRSigstoreSignedKeyData, except it must not fail.
func xNewPRSigstoreSignedKeyData(keyData []byte, signedIdentity PolicyReferenceMatch) PolicyRequirement {
	pr, err := NewPRSigstoreSignedKeyData(keyData, signedIdentity)
	if err != nil {
		panic("xNewPRSigstoreSignedKeyData failed")
	}
	return pr
}

func TestNewPRSigstoreSigned(t *testing.T) {
	const testPath = "/foo/bar"
	testData := []byte("abc")
	testIdentity := NewPRMMatchRepoDigestOrExact()

	// Success
	pr, err := newPRSigstoreSigned(testPath, nil, testIdentity)
	require.NoError(t, err)
	assert.Equal(t, &prSigstoreSigned{
		prCommon:       prCommon{prTypeSigstoreSigned},
		KeyPath:        testPath,
		KeyData:        nil,
		SignedIdentity: testIdentity,
	}, pr)
	pr, err = newPRSigstoreSigned("", testData, testIdentity)
	require.NoError(t, err)
	assert.Equal(t, &prSigstoreSigned{
		prCommon:       prCommon{prTypeSigstoreSigned},
		KeyPath:        "",
		KeyData:        testData,
		SignedIdentity: testIdentity,
	}, pr)

	// Both keyPath and keyData specified
	_, err = newPRSigstoreSigned(testPath, testData, testIdentity)
	assert.Error(t, err)

	// Invalid signedIdentity
	_, err = newPRSigstoreSigned(testPath, nil, nil)
	assert.Error(t, err)
}

func TestNewPRSigstoreSignedKeyPath(t *testing.T) {
	const testPath = "/foo/bar"
	_pr, err := NewPRSigstoreSignedKeyPath(testPath, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	pr, ok := _pr.(*prSigstoreSigned)
	require.True(t, ok)
	assert.Equal(t, testPath, pr.KeyPath)
	// Failure cases tested in TestNewPRSigstoreSigned.
}

func TestNewPRSigstoreSignedKeyData(t *testing.T) {
	testData := []byte("abc")
	_pr, err := NewPRSigstoreSignedKeyData(testData, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	pr, ok := _pr.(*prSigstoreSigned)
	require.True(t, ok)
	assert.Equal(t, testData, pr.KeyData)
	// Failure cases tested in TestNewPRSigstoreSigned.
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
