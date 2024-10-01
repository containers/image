package signature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"

	// this import is needed  where we use the "atomic" transport in TestPolicyUnmarshalJSON
	_ "github.com/containers/image/v5/openshift"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mSA map[string]any // To minimize typing the long name

// A short-hand way to get a JSON object field value or panic. No error handling done, we know
// what we are working with, a panic in a test is good enough, and fitting test cases on a single line
// is a priority.
func x(m mSA, fields ...string) mSA {
	for _, field := range fields {
		// Not .(mSA) because type assertion of an unnamed type to a named type always fails (the types
		// are not "identical"), but the assignment is fine because they are "assignable".
		m = m[field].(map[string]any)
	}
	return m
}

// policyFixtureContents is a data structure equal to the contents of "fixtures/policy.json"
var policyFixtureContents = &Policy{
	Default: PolicyRequirements{NewPRReject()},
	Transports: map[string]PolicyTransportScopes{
		"dir": {
			"": PolicyRequirements{NewPRInsecureAcceptAnything()},
		},
		"docker": {
			"example.com/playground": {
				NewPRInsecureAcceptAnything(),
			},
			"example.com/production": {
				xNewPRSignedByKeyPath(SBKeyTypeGPGKeys,
					"/keys/employee-gpg-keyring",
					NewPRMMatchRepoDigestOrExact()),
			},
			"example.com/hardened": {
				xNewPRSignedByKeyPath(SBKeyTypeGPGKeys,
					"/keys/employee-gpg-keyring",
					NewPRMMatchRepository()),
				xNewPRSignedByKeyPath(SBKeyTypeSignedByGPGKeys,
					"/keys/public-key-signing-gpg-keyring",
					NewPRMMatchExact()),
				xNewPRSignedBaseLayer(xNewPRMExactRepository("registry.access.redhat.com/rhel7/rhel")),
			},
			"example.com/hardened-x509": {
				xNewPRSignedByKeyPath(SBKeyTypeX509Certificates,
					"/keys/employee-cert-file",
					NewPRMMatchRepository()),
				xNewPRSignedByKeyPath(SBKeyTypeSignedByX509CAs,
					"/keys/public-key-signing-ca-file",
					NewPRMMatchRepoDigestOrExact()),
			},
			"registry.access.redhat.com": {
				xNewPRSignedByKeyPath(SBKeyTypeSignedByGPGKeys,
					"/keys/RH-key-signing-key-gpg-keyring",
					NewPRMMatchRepoDigestOrExact()),
			},
			"registry.redhat.io/beta": {
				xNewPRSignedByKeyPaths(SBKeyTypeGPGKeys,
					[]string{"/keys/RH-production-signing-key-gpg-keyring", "/keys/RH-beta-signing-key-gpg-keyring"},
					newPRMMatchRepoDigestOrExact()),
			},
			"private-mirror:5000/vendor-mirror": {
				xNewPRSignedByKeyPath(SBKeyTypeGPGKeys,
					"/keys/vendor-gpg-keyring",
					xNewPRMRemapIdentity("private-mirror:5000/vendor-mirror", "vendor.example.com")),
			},
			"*.access.redhat.com": {
				xNewPRSignedByKeyPath(SBKeyTypeSignedByGPGKeys,
					"/keys/RH-key-signing-key-gpg-keyring",
					NewPRMMatchRepoDigestOrExact()),
			},
			"*.redhat.com": {
				xNewPRSignedByKeyPath(SBKeyTypeSignedByGPGKeys,
					"/keys/RH-key-signing-key-gpg-keyring",
					NewPRMMatchRepoDigestOrExact()),
			},
			"*.com": {
				xNewPRSignedByKeyPath(SBKeyTypeSignedByGPGKeys,
					"/keys/RH-key-signing-key-gpg-keyring",
					NewPRMMatchRepoDigestOrExact()),
			},
			"bogus/key-data-example": {
				xNewPRSignedByKeyData(SBKeyTypeSignedByGPGKeys,
					[]byte("nonsense"),
					NewPRMMatchRepoDigestOrExact()),
			},
			"bogus/signed-identity-example": {
				xNewPRSignedBaseLayer(xNewPRMExactReference("registry.access.redhat.com/rhel7/rhel:latest")),
			},
			"example.com/sigstore/key-data-example": {
				xNewPRSigstoreSigned(
					PRSigstoreSignedWithKeyData([]byte("nonsense")),
					PRSigstoreSignedWithSignedIdentity(NewPRMMatchRepoDigestOrExact()),
				),
			},
			"example.com/sigstore/key-path-example": {
				xNewPRSigstoreSigned(
					PRSigstoreSignedWithKeyPath("/keys/public-key"),
					PRSigstoreSignedWithSignedIdentity(NewPRMMatchRepository()),
				),
			},
		},
	},
}

func TestInvalidPolicyFormatError(t *testing.T) {
	// A stupid test just to keep code coverage
	s := "test"
	err := InvalidPolicyFormatError(s)
	assert.Equal(t, s, err.Error())
}

func TestDefaultPolicy(t *testing.T) {
	// We can't test the actual systemDefaultPolicyPath, so override.
	// TestDefaultPolicyPath below tests that we handle the overrides and defaults
	// correctly.

	// Success
	policy, err := DefaultPolicy(&types.SystemContext{SignaturePolicyPath: "./fixtures/policy.json"})
	require.NoError(t, err)
	assert.Equal(t, policyFixtureContents, policy)

	for _, path := range []string{
		"/this/does/not/exist", // Error reading file
		"/dev/null",            // A failure case; most are tested in the individual method unit tests.
	} {
		policy, err := DefaultPolicy(&types.SystemContext{SignaturePolicyPath: path})
		assert.Error(t, err)
		assert.Nil(t, policy)
	}
}

func TestDefaultPolicyPath(t *testing.T) {
	const nondefaultPath = "/this/is/not/the/default/path.json"
	const variableReference = "$HOME"
	const rootPrefix = "/root/prefix"
	tempHome := t.TempDir()
	userDefaultPolicyPath := filepath.Join(tempHome, userPolicyFile)
	tempsystemdefaultpath := filepath.Join(tempHome, systemDefaultPolicyPath)
	for _, c := range []struct {
		sys               *types.SystemContext
		userfilePresent   bool
		systemfilePresent bool
		expected          string
		expectedError     string
	}{
		// The common case
		{nil, false, true, tempsystemdefaultpath, ""},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, false, true, tempsystemdefaultpath, ""},
		// Path overridden
		{&types.SystemContext{SignaturePolicyPath: nondefaultPath}, false, true, nondefaultPath, ""},
		// Root overridden
		{
			&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix},
			false,
			true,
			filepath.Join(rootPrefix, tempsystemdefaultpath),
			"",
		},
		// Empty context and user policy present
		{&types.SystemContext{}, true, true, userDefaultPolicyPath, ""},
		// Only user policy present
		{nil, true, true, userDefaultPolicyPath, ""},
		// Context signature path and user policy present
		{
			&types.SystemContext{
				SignaturePolicyPath: nondefaultPath,
			},
			true,
			true,
			nondefaultPath,
			"",
		},
		// Root and user policy present
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			true,
			true,
			userDefaultPolicyPath,
			"",
		},
		// Context and user policy file preset simultaneously
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				SignaturePolicyPath:          nondefaultPath,
			},
			true,
			true,
			nondefaultPath,
			"",
		},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				SignaturePolicyPath:          nondefaultPath,
			},
			false,
			true,
			nondefaultPath,
			"",
		},
		// No environment expansion happens in the overridden paths
		{&types.SystemContext{SignaturePolicyPath: variableReference}, false, true, variableReference, ""},
		// No policy.json file is present in userfilePath and systemfilePath
		{nil, false, false, "", fmt.Sprintf("no policy.json file found at any of the following: %q, %q", userDefaultPolicyPath, tempsystemdefaultpath)},
	} {
		paths := []struct {
			condition bool
			path      string
		}{
			{c.userfilePresent, userDefaultPolicyPath},
			{c.systemfilePresent, tempsystemdefaultpath},
		}
		for _, p := range paths {
			if p.condition {
				err := os.MkdirAll(filepath.Dir(p.path), os.ModePerm)
				require.NoError(t, err)
				f, err := os.Create(p.path)
				require.NoError(t, err)
				f.Close()
			} else {
				os.Remove(p.path)
			}
		}
		path, err := defaultPolicyPathWithHomeDir(c.sys, tempHome, tempsystemdefaultpath)
		if c.expectedError != "" {
			assert.Empty(t, path)
			assert.EqualError(t, err, c.expectedError)
		} else {
			require.NoError(t, err)
			assert.Equal(t, c.expected, path)
		}
	}
}

func TestNewPolicyFromFile(t *testing.T) {
	// Success
	policy, err := NewPolicyFromFile("./fixtures/policy.json")
	require.NoError(t, err)
	assert.Equal(t, policyFixtureContents, policy)

	// Error reading file
	_, err = NewPolicyFromFile("/this/does/not/exist")
	assert.Error(t, err)

	// A failure case; most are tested in the individual method unit tests.
	_, err = NewPolicyFromFile("/dev/null")
	require.Error(t, err)
	var formatError InvalidPolicyFormatError
	assert.ErrorAs(t, err, &formatError)
}

func TestNewPolicyFromBytes(t *testing.T) {
	// Success
	bytes, err := os.ReadFile("./fixtures/policy.json")
	require.NoError(t, err)
	policy, err := NewPolicyFromBytes(bytes)
	require.NoError(t, err)
	assert.Equal(t, policyFixtureContents, policy)

	// A failure case; most are tested in the individual method unit tests.
	_, err = NewPolicyFromBytes([]byte(""))
	require.Error(t, err)
	assert.IsType(t, InvalidPolicyFormatError(""), err)
}

// FIXME? There is quite a bit of duplication below. Factor some of it out?

// jsonUnmarshalFromObject is like json.Unmarshal(), but the input is an arbitrary object
// that is JSON-marshalled first (as a convenient way to create an invalid/unusual JSON input)
func jsonUnmarshalFromObject(t *testing.T, object any, dest any) error {
	testJSON, err := json.Marshal(object)
	require.NoError(t, err)
	return json.Unmarshal(testJSON, dest)
}

// assertJSONUnmarshalFromObjectFails checks that unmarshaling the JSON-marshaled version
// of an arbitrary object (as a convenient way to create an invalid/unusual JSON input) into
// dest fails.
func assertJSONUnmarshalFromObjectFails(t *testing.T, object any, dest any) {
	err := jsonUnmarshalFromObject(t, object, dest)
	assert.Error(t, err)
}

// testInvalidJSONInput verifies that obviously invalid input is rejected for dest.
func testInvalidJSONInput(t *testing.T, dest json.Unmarshaler) {
	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	err := json.Unmarshal([]byte("&"), dest)
	assert.Error(t, err)
	err = dest.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object/array/string
	err = json.Unmarshal([]byte("1"), dest)
	assert.Error(t, err)
}

// addExtraJSONMember adds an additional member "$name": $extra,
// possibly with a duplicate name, to encoded.
// Errors, if any, are reported through t.
func addExtraJSONMember(t *testing.T, encoded []byte, name string, extra any) []byte {
	extraJSON, err := json.Marshal(extra)
	require.NoError(t, err)

	preserved, ok := bytes.CutSuffix(encoded, []byte("}"))
	require.True(t, ok)

	res := bytes.Join([][]byte{preserved, []byte(`,"`), []byte(name), []byte(`":`), extraJSON, []byte("}")}, nil)
	// Verify that the result is valid JSON, as a sanity check that we are actually triggering
	// the “duplicate member” case in the caller.
	var raw map[string]any
	err = json.Unmarshal(res, &raw)
	require.NoError(t, err)
	return res
}

// policyJSONUnmarshallerTests formalizes the repeated structure of the JSON unmarshaller
// tests in this file, and allows sharing the test implementation.
type policyJSONUmarshallerTests[T any] struct {
	newDest         func() json.Unmarshaler // Create a new json.Unmarshaler to test against
	newValidObject  func() (T, error)       // A function that generates a valid object, used as a base for other tests
	otherJSONParser func([]byte) (T, error) // Another function that must accept the result of encoding validObject
	invalidObjects  []mSA                   // mSA values that are invalid for this unmarshaller; a simpler alternative to breakFns
	breakFns        []func(mSA)             // Functions that edit a mSA from newValidObject() to make it invalid
	duplicateFields []string                // Names of fields in the return value of newValidObject() that should not be duplicated
}

// validObjectAndJSON returns an object created by d.newValidObject() and its JSON representation.
func (d policyJSONUmarshallerTests[T]) validObjectAndJSON(t *testing.T) (T, []byte) {
	validObject, err := d.newValidObject()
	require.NoError(t, err)
	validJSON, err := json.Marshal(validObject)
	require.NoError(t, err)
	return validObject, validJSON
}

func (d policyJSONUmarshallerTests[T]) run(t *testing.T) {
	dest := d.newDest()
	testInvalidJSONInput(t, dest)

	validObject, validJSON := d.validObjectAndJSON(t)

	// Success
	dest = d.newDest()
	err := json.Unmarshal(validJSON, dest)
	require.NoError(t, err)
	assert.Equal(t, validObject, dest)

	// otherJSONParser recognizes this data
	if d.otherJSONParser != nil {
		other, err := d.otherJSONParser(validJSON)
		require.NoError(t, err)
		assert.Equal(t, validObject, other)
	}

	// Invalid JSON objects
	for _, invalid := range d.invalidObjects {
		dest := d.newDest()
		assertJSONUnmarshalFromObjectFails(t, invalid, dest)
	}
	// Various ways to corrupt the JSON
	for index, fn := range d.breakFns {
		t.Run(fmt.Sprintf("breakFns[%d]", index), func(t *testing.T) {
			var tmp mSA
			err := json.Unmarshal(validJSON, &tmp)
			require.NoError(t, err)

			fn(tmp)

			dest := d.newDest()
			assertJSONUnmarshalFromObjectFails(t, tmp, dest)
		})
	}

	// Duplicated fields
	for _, field := range d.duplicateFields {
		var tmp mSA
		err := json.Unmarshal(validJSON, &tmp)
		require.NoError(t, err)

		testJSON := addExtraJSONMember(t, validJSON, field, tmp[field])

		dest := d.newDest()
		err = json.Unmarshal(testJSON, dest)
		assert.Error(t, err)
	}
}

// xNewPRSignedByKeyPath is like NewPRSignedByKeyPath, except it must not fail.
func xNewPRSignedByKeyPath(keyType sbKeyType, keyPath string, signedIdentity PolicyReferenceMatch) PolicyRequirement {
	pr, err := NewPRSignedByKeyPath(keyType, keyPath, signedIdentity)
	if err != nil {
		panic("xNewPRSignedByKeyPath failed")
	}
	return pr
}

// xNewPRSignedByKeyPaths is like NewPRSignedByKeyPaths, except it must not fail.
func xNewPRSignedByKeyPaths(keyType sbKeyType, keyPaths []string, signedIdentity PolicyReferenceMatch) PolicyRequirement {
	pr, err := NewPRSignedByKeyPaths(keyType, keyPaths, signedIdentity)
	if err != nil {
		panic("xNewPRSignedByKeyPaths failed")
	}
	return pr
}

// xNewPRSignedByKeyData is like NewPRSignedByKeyData, except it must not fail.
func xNewPRSignedByKeyData(keyType sbKeyType, keyData []byte, signedIdentity PolicyReferenceMatch) PolicyRequirement {
	pr, err := NewPRSignedByKeyData(keyType, keyData, signedIdentity)
	if err != nil {
		panic("xNewPRSignedByKeyData failed")
	}
	return pr
}

func TestPolicyUnmarshalJSON(t *testing.T) {
	tests := policyJSONUmarshallerTests[*Policy]{
		newDest: func() json.Unmarshaler { return &Policy{} },
		newValidObject: func() (*Policy, error) {
			return &Policy{
				Default: []PolicyRequirement{
					xNewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("abc"), NewPRMMatchRepoDigestOrExact()),
				},
				Transports: map[string]PolicyTransportScopes{
					"docker": {
						"docker.io/library/busybox": []PolicyRequirement{
							xNewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("def"), NewPRMMatchRepoDigestOrExact()),
						},
						"registry.access.redhat.com": []PolicyRequirement{
							xNewPRSignedByKeyData(SBKeyTypeSignedByGPGKeys, []byte("RH"), NewPRMMatchRepository()),
						},
					},
					"atomic": {
						"registry.access.redhat.com/rhel7": []PolicyRequirement{
							xNewPRSignedByKeyData(SBKeyTypeSignedByGPGKeys, []byte("RHatomic"), NewPRMMatchRepository()),
						},
					},
					"unknown": {
						"registry.access.redhat.com/rhel7": []PolicyRequirement{
							xNewPRSignedByKeyData(SBKeyTypeSignedByGPGKeys, []byte("RHatomic"), NewPRMMatchRepository()),
						},
					},
				},
			}, nil
		},
		otherJSONParser: nil,
		breakFns: []func(mSA){
			// The "default" field is missing
			func(v mSA) { delete(v, "default") },
			// Extra top-level sub-object
			func(v mSA) { v["unexpected"] = 1 },
			// "default" not an array
			func(v mSA) { v["default"] = 1 },
			func(v mSA) { v["default"] = mSA{} },
			// "transports" not an object
			func(v mSA) { v["transports"] = 1 },
			func(v mSA) { v["transports"] = []string{} },
			// "default" is an invalid PolicyRequirements
			func(v mSA) { v["default"] = PolicyRequirements{} },
		},
		duplicateFields: []string{"default", "transports"},
	}
	tests.run(t)

	// Various allowed modifications to the policy
	_, validJSON := tests.validObjectAndJSON(t)
	allowedModificationFns := []func(mSA){
		// Delete the map of transport-specific scopes
		func(v mSA) { delete(v, "transports") },
		// Use an empty map of transport-specific scopes
		func(v mSA) { v["transports"] = map[string]PolicyTransportScopes{} },
	}
	for _, fn := range allowedModificationFns {
		var tmp mSA
		err := json.Unmarshal(validJSON, &tmp)
		require.NoError(t, err)

		fn(tmp)

		p := Policy{}
		err = jsonUnmarshalFromObject(t, tmp, &p)
		assert.NoError(t, err)
	}
}

func TestPolicyTransportScopesUnmarshalJSON(t *testing.T) {
	// Start with a valid JSON.
	validPTS := PolicyTransportScopes{
		"": []PolicyRequirement{
			xNewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("global"), NewPRMMatchRepoDigestOrExact()),
		},
	}

	// Nothing can be unmarshaled directly into PolicyTransportScopes
	pts := PolicyTransportScopes{}
	assertJSONUnmarshalFromObjectFails(t, validPTS, &pts)
}

// Return the result of modifying validJSON with fn and unmarshaling it into *pts
// using transport.
func tryUnmarshalModifiedPTS(t *testing.T, pts *PolicyTransportScopes, transport types.ImageTransport,
	validJSON []byte, modifyFn func(mSA)) error {
	var tmp mSA
	err := json.Unmarshal(validJSON, &tmp)
	require.NoError(t, err)

	modifyFn(tmp)

	*pts = PolicyTransportScopes{}
	dest := policyTransportScopesWithTransport{
		transport: transport,
		dest:      pts,
	}
	return jsonUnmarshalFromObject(t, tmp, &dest)
}

func TestPolicyTransportScopesWithTransportUnmarshalJSON(t *testing.T) {
	var pts PolicyTransportScopes

	dest := policyTransportScopesWithTransport{
		transport: docker.Transport,
		dest:      &pts,
	}
	testInvalidJSONInput(t, &dest)

	// Start with a valid JSON.
	validPTS := PolicyTransportScopes{
		"docker.io/library/busybox": []PolicyRequirement{
			xNewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("def"), NewPRMMatchRepoDigestOrExact()),
		},
		"registry.access.redhat.com": []PolicyRequirement{
			xNewPRSignedByKeyData(SBKeyTypeSignedByGPGKeys, []byte("RH"), NewPRMMatchRepository()),
		},
		"": []PolicyRequirement{
			xNewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("global"), NewPRMMatchRepoDigestOrExact()),
		},
	}
	validJSON, err := json.Marshal(validPTS)
	require.NoError(t, err)

	// Success
	pts = PolicyTransportScopes{}
	dest = policyTransportScopesWithTransport{
		transport: docker.Transport,
		dest:      &pts,
	}
	err = json.Unmarshal(validJSON, &dest)
	require.NoError(t, err)
	assert.Equal(t, validPTS, pts)

	// Various ways to corrupt the JSON
	breakFns := []func(mSA){
		// A scope is not an array
		func(v mSA) { v["docker.io/library/busybox"] = 1 },
		func(v mSA) { v["docker.io/library/busybox"] = mSA{} },
		func(v mSA) { v[""] = 1 },
		func(v mSA) { v[""] = mSA{} },
		// A scope is an invalid PolicyRequirements
		func(v mSA) { v["docker.io/library/busybox"] = PolicyRequirements{} },
		func(v mSA) { v[""] = PolicyRequirements{} },
	}
	for _, fn := range breakFns {
		err = tryUnmarshalModifiedPTS(t, &pts, docker.Transport, validJSON, fn)
		assert.Error(t, err)
	}

	// Duplicated fields
	for _, field := range []string{"docker.io/library/busybox", ""} {
		var tmp mSA
		err := json.Unmarshal(validJSON, &tmp)
		require.NoError(t, err)

		testJSON := addExtraJSONMember(t, validJSON, field, tmp[field])

		pts = PolicyTransportScopes{}
		dest := policyTransportScopesWithTransport{
			transport: docker.Transport,
			dest:      &pts,
		}
		err = json.Unmarshal(testJSON, &dest)
		assert.Error(t, err)
	}

	// Scope rejected by transport the Docker scopes we use as valid are rejected by directory.Transport
	// as relative paths.
	err = tryUnmarshalModifiedPTS(t, &pts, directory.Transport, validJSON,
		func(v mSA) {})
	assert.Error(t, err)

	// Various allowed modifications to the policy
	allowedModificationFns := []func(mSA){
		// The "" scope is missing
		func(v mSA) { delete(v, "") },
		// The policy is completely empty
		func(v mSA) { clear(v) },
	}
	for _, fn := range allowedModificationFns {
		err = tryUnmarshalModifiedPTS(t, &pts, docker.Transport, validJSON, fn)
		require.NoError(t, err)
	}
}

func TestPolicyRequirementsUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[*PolicyRequirements]{
		newDest: func() json.Unmarshaler { return &PolicyRequirements{} },
		newValidObject: func() (*PolicyRequirements, error) {
			return &PolicyRequirements{
				xNewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("def"), NewPRMMatchRepoDigestOrExact()),
				xNewPRSignedByKeyData(SBKeyTypeSignedByGPGKeys, []byte("RH"), NewPRMMatchRepository()),
			}, nil
		},
		otherJSONParser: nil,
	}.run(t)

	// This would be inconvenient to integrate into policyJSONUnmarshallerTests.invalidObjects
	// because all other users are easier to express as mSA.
	for _, invalid := range [][]any{
		// No requirements
		{},
		// A member is not an object
		{1},
		// A member has an invalid type
		{prSignedBy{prCommon: prCommon{Type: "this is invalid"}}},
		// A member has a valid type but invalid contents
		{prSignedBy{
			prCommon: prCommon{Type: prTypeSignedBy},
			KeyType:  "this is invalid",
		}},
	} {
		reqs := PolicyRequirements{}
		assertJSONUnmarshalFromObjectFails(t, invalid, &reqs)
	}
}

func TestNewPolicyRequirementFromJSON(t *testing.T) {
	// Sample success. Others tested in the individual PolicyRequirement.UnmarshalJSON implementations.
	validReq := NewPRInsecureAcceptAnything()
	validJSON, err := json.Marshal(validReq)
	require.NoError(t, err)
	req, err := newPolicyRequirementFromJSON(validJSON)
	require.NoError(t, err)
	assert.Equal(t, validReq, req)

	// Invalid
	for _, invalid := range []any{
		// Not an object
		1,
		// Missing type
		prCommon{},
		// Invalid type
		prCommon{Type: "this is invalid"},
		// Valid type but invalid contents
		prSignedBy{
			prCommon: prCommon{Type: prTypeSignedBy},
			KeyType:  "this is invalid",
		},
	} {
		testJSON, err := json.Marshal(invalid)
		require.NoError(t, err)

		_, err = newPolicyRequirementFromJSON(testJSON)
		assert.Error(t, err, string(testJSON))
	}
}

func TestNewPRInsecureAcceptAnything(t *testing.T) {
	_pr := NewPRInsecureAcceptAnything()
	pr, ok := _pr.(*prInsecureAcceptAnything)
	require.True(t, ok)
	assert.Equal(t, &prInsecureAcceptAnything{prCommon{prTypeInsecureAcceptAnything}}, pr)
}

func TestPRInsecureAcceptAnythingUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyRequirement]{
		newDest: func() json.Unmarshaler { return &prInsecureAcceptAnything{} },
		newValidObject: func() (PolicyRequirement, error) {
			return NewPRInsecureAcceptAnything(), nil
		},
		otherJSONParser: newPolicyRequirementFromJSON,
		invalidObjects: []mSA{
			// Missing "type" field
			{},
			// Wrong "type" field
			{"type": 1},
			{"type": "this is invalid"},
			// Extra fields
			{
				"type":    string(prTypeInsecureAcceptAnything),
				"unknown": "foo",
			},
		},
		duplicateFields: []string{"type"},
	}.run(t)
}

func TestNewPRReject(t *testing.T) {
	_pr := NewPRReject()
	pr, ok := _pr.(*prReject)
	require.True(t, ok)
	assert.Equal(t, &prReject{prCommon{prTypeReject}}, pr)
}

func TestPRRejectUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyRequirement]{
		newDest: func() json.Unmarshaler { return &prReject{} },
		newValidObject: func() (PolicyRequirement, error) {
			return NewPRReject(), nil
		},
		otherJSONParser: newPolicyRequirementFromJSON,
		invalidObjects: []mSA{
			// Missing "type" field
			{},
			// Wrong "type" field
			{"type": 1},
			{"type": "this is invalid"},
			// Extra fields
			{
				"type":    string(prTypeReject),
				"unknown": "foo",
			},
		},
		duplicateFields: []string{"type"},
	}.run(t)
}

func TestNewPRSignedBy(t *testing.T) {
	const testPath = "/foo/bar"
	testPaths := []string{"/path/1", "/path/2"}
	testData := []byte("abc")
	testIdentity := NewPRMMatchRepoDigestOrExact()

	// Success
	pr, err := newPRSignedBy(SBKeyTypeGPGKeys, testPath, nil, nil, testIdentity)
	require.NoError(t, err)
	assert.Equal(t, &prSignedBy{
		prCommon:       prCommon{prTypeSignedBy},
		KeyType:        SBKeyTypeGPGKeys,
		KeyPath:        testPath,
		KeyPaths:       nil,
		KeyData:        nil,
		SignedIdentity: testIdentity,
	}, pr)
	pr, err = newPRSignedBy(SBKeyTypeGPGKeys, "", testPaths, nil, testIdentity)
	require.NoError(t, err)
	assert.Equal(t, &prSignedBy{
		prCommon:       prCommon{prTypeSignedBy},
		KeyType:        SBKeyTypeGPGKeys,
		KeyPath:        "",
		KeyPaths:       testPaths,
		KeyData:        nil,
		SignedIdentity: testIdentity,
	}, pr)
	pr, err = newPRSignedBy(SBKeyTypeGPGKeys, "", nil, testData, testIdentity)
	require.NoError(t, err)
	assert.Equal(t, &prSignedBy{
		prCommon:       prCommon{prTypeSignedBy},
		KeyType:        SBKeyTypeGPGKeys,
		KeyPath:        "",
		KeyPaths:       nil,
		KeyData:        testData,
		SignedIdentity: testIdentity,
	}, pr)

	// Invalid keyType
	_, err = newPRSignedBy(sbKeyType(""), testPath, nil, nil, testIdentity)
	assert.Error(t, err)
	_, err = newPRSignedBy(sbKeyType("this is invalid"), testPath, nil, nil, testIdentity)
	assert.Error(t, err)

	// Invalid keyPath/keyPaths/keyData combinations
	_, err = newPRSignedBy(SBKeyTypeGPGKeys, testPath, testPaths, testData, testIdentity)
	assert.Error(t, err)
	_, err = newPRSignedBy(SBKeyTypeGPGKeys, testPath, testPaths, nil, testIdentity)
	assert.Error(t, err)
	_, err = newPRSignedBy(SBKeyTypeGPGKeys, testPath, nil, testData, testIdentity)
	assert.Error(t, err)
	_, err = newPRSignedBy(SBKeyTypeGPGKeys, "", testPaths, testData, testIdentity)
	assert.Error(t, err)
	_, err = newPRSignedBy(SBKeyTypeGPGKeys, "", nil, nil, testIdentity)
	assert.Error(t, err)

	// Invalid signedIdentity
	_, err = newPRSignedBy(SBKeyTypeGPGKeys, testPath, nil, nil, nil)
	assert.Error(t, err)
}

func TestNewPRSignedByKeyPath(t *testing.T) {
	const testPath = "/foo/bar"
	_pr, err := NewPRSignedByKeyPath(SBKeyTypeGPGKeys, testPath, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	pr, ok := _pr.(*prSignedBy)
	require.True(t, ok)
	assert.Equal(t, testPath, pr.KeyPath)
	// Failure cases tested in TestNewPRSignedBy.
}

func TestNewPRSignedByKeyPaths(t *testing.T) {
	testPaths := []string{"/path/1", "/path/2"}
	_pr, err := NewPRSignedByKeyPaths(SBKeyTypeGPGKeys, testPaths, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	pr, ok := _pr.(*prSignedBy)
	require.True(t, ok)
	assert.Equal(t, testPaths, pr.KeyPaths)
	// Failure cases tested in TestNewPRSignedBy.
}

func TestNewPRSignedByKeyData(t *testing.T) {
	testData := []byte("abc")
	_pr, err := NewPRSignedByKeyData(SBKeyTypeGPGKeys, testData, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	pr, ok := _pr.(*prSignedBy)
	require.True(t, ok)
	assert.Equal(t, testData, pr.KeyData)
	// Failure cases tested in TestNewPRSignedBy.
}

// Return the result of modifying validJSON with fn and unmarshaling it into *pr
func tryUnmarshalModifiedSignedBy(t *testing.T, pr *prSignedBy, validJSON []byte, modifyFn func(mSA)) error {
	var tmp mSA
	err := json.Unmarshal(validJSON, &tmp)
	require.NoError(t, err)

	modifyFn(tmp)

	*pr = prSignedBy{}
	return jsonUnmarshalFromObject(t, tmp, &pr)
}

func TestPRSignedByUnmarshalJSON(t *testing.T) {
	keyDataTests := policyJSONUmarshallerTests[PolicyRequirement]{
		newDest: func() json.Unmarshaler { return &prSignedBy{} },
		newValidObject: func() (PolicyRequirement, error) {
			return NewPRSignedByKeyData(SBKeyTypeGPGKeys, []byte("abc"), NewPRMMatchRepoDigestOrExact())
		},
		otherJSONParser: newPolicyRequirementFromJSON,
		breakFns: []func(mSA){
			// The "type" field is missing
			func(v mSA) { delete(v, "type") },
			// Wrong "type" field
			func(v mSA) { v["type"] = 1 },
			func(v mSA) { v["type"] = "this is invalid" },
			// Extra top-level sub-object
			func(v mSA) { v["unexpected"] = 1 },
			// The "keyType" field is missing
			func(v mSA) { delete(v, "keyType") },
			// Invalid "keyType" field
			func(v mSA) { v["keyType"] = "this is invalid" },
			// All three of "keyPath", "keyPaths" and "keyData" are missing
			func(v mSA) { delete(v, "keyData") },
			// All three of "keyPath", "keyPaths" and "keyData" are present
			func(v mSA) { v["keyPath"] = "/foo/bar"; v["keyPaths"] = []string{"/1", "/2"} },
			// Two of "keyPath", "keyPaths" and "keyData" are present
			func(v mSA) { v["keyPath"] = "/foo/bar"; v["keyPaths"] = []string{"/1", "/2"}; delete(v, "keyData") },
			func(v mSA) { v["keyPath"] = "/foo/bar" },
			func(v mSA) { v["keyPaths"] = []string{"/1", "/2"} },
			// Invalid "keyPath" field
			func(v mSA) { delete(v, "keyData"); v["keyPath"] = 1 },
			// Invalid "keyPaths" field
			func(v mSA) { delete(v, "keyData"); v["keyPaths"] = 1 },
			func(v mSA) { delete(v, "keyData"); v["keyPaths"] = []int{1} },
			// Invalid "keyData" field
			func(v mSA) { v["keyData"] = 1 },
			func(v mSA) { v["keyData"] = "this is invalid base64" },
			// Invalid "signedIdentity" field
			func(v mSA) { v["signedIdentity"] = "this is invalid" },
			// "signedIdentity" an explicit nil
			func(v mSA) { v["signedIdentity"] = nil },
		},
		duplicateFields: []string{"type", "keyType", "keyData", "signedIdentity"},
	}
	keyDataTests.run(t)
	// Test the keyPath-specific aspects
	policyJSONUmarshallerTests[PolicyRequirement]{
		newDest: func() json.Unmarshaler { return &prSignedBy{} },
		newValidObject: func() (PolicyRequirement, error) {
			return NewPRSignedByKeyPath(SBKeyTypeGPGKeys, "/foo/bar", NewPRMMatchRepoDigestOrExact())
		},
		otherJSONParser: newPolicyRequirementFromJSON,
		duplicateFields: []string{"type", "keyType", "keyPath", "signedIdentity"},
	}.run(t)
	// Test the keyPaths-specific aspects
	policyJSONUmarshallerTests[PolicyRequirement]{
		newDest: func() json.Unmarshaler { return &prSignedBy{} },
		newValidObject: func() (PolicyRequirement, error) {
			return NewPRSignedByKeyPaths(SBKeyTypeGPGKeys, []string{"/1", "/2"}, NewPRMMatchRepoDigestOrExact())
		},
		otherJSONParser: newPolicyRequirementFromJSON,
		duplicateFields: []string{"type", "keyType", "keyPaths", "signedIdentity"},
	}.run(t)

	var pr prSignedBy

	// Start with a valid JSON.
	_, validJSON := keyDataTests.validObjectAndJSON(t)

	// Various allowed modifications to the requirement
	allowedModificationFns := []func(mSA){
		// Delete the signedIdentity field
		func(v mSA) { delete(v, "signedIdentity") },
	}
	for _, fn := range allowedModificationFns {
		err := tryUnmarshalModifiedSignedBy(t, &pr, validJSON, fn)
		require.NoError(t, err)
	}

	// Various ways to set signedIdentity to the default value
	signedIdentityDefaultFns := []func(mSA){
		// Set signedIdentity to the default explicitly
		func(v mSA) { v["signedIdentity"] = NewPRMMatchRepoDigestOrExact() },
		// Delete the signedIdentity field
		func(v mSA) { delete(v, "signedIdentity") },
	}
	for _, fn := range signedIdentityDefaultFns {
		err := tryUnmarshalModifiedSignedBy(t, &pr, validJSON, fn)
		require.NoError(t, err)
		assert.Equal(t, NewPRMMatchRepoDigestOrExact(), pr.SignedIdentity)
	}
}

func TestSBKeyTypeIsValid(t *testing.T) {
	// Valid values
	for _, s := range []sbKeyType{
		SBKeyTypeGPGKeys,
		SBKeyTypeSignedByGPGKeys,
		SBKeyTypeX509Certificates,
		SBKeyTypeSignedByX509CAs,
	} {
		assert.True(t, s.IsValid())
	}

	// Invalid values
	for _, s := range []string{"", "this is invalid"} {
		assert.False(t, sbKeyType(s).IsValid())
	}
}

func TestSBKeyTypeUnmarshalJSON(t *testing.T) {
	var kt sbKeyType

	testInvalidJSONInput(t, &kt)

	// Valid values.
	for _, v := range []sbKeyType{
		SBKeyTypeGPGKeys,
		SBKeyTypeSignedByGPGKeys,
		SBKeyTypeX509Certificates,
		SBKeyTypeSignedByX509CAs,
	} {
		kt = sbKeyType("")
		err := json.Unmarshal([]byte(`"`+string(v)+`"`), &kt)
		assert.NoError(t, err)
	}

	// Invalid values
	kt = sbKeyType("")
	err := json.Unmarshal([]byte(`""`), &kt)
	assert.Error(t, err)

	kt = sbKeyType("")
	err = json.Unmarshal([]byte(`"this is invalid"`), &kt)
	assert.Error(t, err)
}

// NewPRSignedBaseLayer is like NewPRSignedBaseLayer, except it must not fail.
func xNewPRSignedBaseLayer(baseLayerIdentity PolicyReferenceMatch) PolicyRequirement {
	pr, err := NewPRSignedBaseLayer(baseLayerIdentity)
	if err != nil {
		panic("xNewPRSignedBaseLayer failed")
	}
	return pr
}

func TestNewPRSignedBaseLayer(t *testing.T) {
	testBLI := NewPRMMatchExact()

	// Success
	_pr, err := NewPRSignedBaseLayer(testBLI)
	require.NoError(t, err)
	pr, ok := _pr.(*prSignedBaseLayer)
	require.True(t, ok)
	assert.Equal(t, &prSignedBaseLayer{
		prCommon:          prCommon{prTypeSignedBaseLayer},
		BaseLayerIdentity: testBLI,
	}, pr)

	// Invalid baseLayerIdentity
	_, err = NewPRSignedBaseLayer(nil)
	assert.Error(t, err)
}

func TestPRSignedBaseLayerUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyRequirement]{
		newDest: func() json.Unmarshaler { return &prSignedBaseLayer{} },
		newValidObject: func() (PolicyRequirement, error) {
			baseIdentity, err := NewPRMExactReference("registry.access.redhat.com/rhel7/rhel:7.2.3")
			require.NoError(t, err)
			return NewPRSignedBaseLayer(baseIdentity)
		},
		otherJSONParser: newPolicyRequirementFromJSON,
		breakFns: []func(mSA){
			// The "type" field is missing
			func(v mSA) { delete(v, "type") },
			// Wrong "type" field
			func(v mSA) { v["type"] = 1 },
			func(v mSA) { v["type"] = "this is invalid" },
			// Extra top-level sub-object
			func(v mSA) { v["unexpected"] = 1 },
			// The "baseLayerIdentity" field is missing
			func(v mSA) { delete(v, "baseLayerIdentity") },
			// Invalid "baseLayerIdentity" field
			func(v mSA) { v["baseLayerIdentity"] = "this is invalid" },
			// Invalid "baseLayerIdentity" an explicit nil
			func(v mSA) { v["baseLayerIdentity"] = nil },
		},
		duplicateFields: []string{"type", "baseLayerIdentity"},
	}.run(t)
}

func TestNewPolicyReferenceMatchFromJSON(t *testing.T) {
	// Sample success. Others tested in the individual PolicyReferenceMatch.UnmarshalJSON implementations.
	validPRM := NewPRMMatchRepoDigestOrExact()
	validJSON, err := json.Marshal(validPRM)
	require.NoError(t, err)
	prm, err := newPolicyReferenceMatchFromJSON(validJSON)
	require.NoError(t, err)
	assert.Equal(t, validPRM, prm)

	// Invalid
	for _, invalid := range []any{
		// Not an object
		1,
		// Missing type
		prmCommon{},
		// Invalid type
		prmCommon{Type: "this is invalid"},
		// Valid type but invalid contents
		prmExactReference{
			prmCommon:       prmCommon{Type: prmTypeExactReference},
			DockerReference: "",
		},
	} {
		testJSON, err := json.Marshal(invalid)
		require.NoError(t, err)

		_, err = newPolicyReferenceMatchFromJSON(testJSON)
		assert.Error(t, err, string(testJSON))
	}
}

func TestNewPRMMatchExact(t *testing.T) {
	_prm := NewPRMMatchExact()
	prm, ok := _prm.(*prmMatchExact)
	require.True(t, ok)
	assert.Equal(t, &prmMatchExact{prmCommon{prmTypeMatchExact}}, prm)
}

func TestPRMMatchExactUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyReferenceMatch]{
		newDest: func() json.Unmarshaler { return &prmMatchExact{} },
		newValidObject: func() (PolicyReferenceMatch, error) {
			return NewPRMMatchExact(), nil
		},
		otherJSONParser: newPolicyReferenceMatchFromJSON,
		invalidObjects: []mSA{
			// Missing "type" field
			{},
			// Wrong "type" field
			{"type": 1},
			{"type": "this is invalid"},
			// Extra fields
			{
				"type":    string(prmTypeMatchExact),
				"unknown": "foo",
			},
		},
		duplicateFields: []string{"type"},
	}.run(t)
}

func TestNewPRMMatchRepoDigestOrExact(t *testing.T) {
	_prm := NewPRMMatchRepoDigestOrExact()
	prm, ok := _prm.(*prmMatchRepoDigestOrExact)
	require.True(t, ok)
	assert.Equal(t, &prmMatchRepoDigestOrExact{prmCommon{prmTypeMatchRepoDigestOrExact}}, prm)
}

func TestPRMMatchRepoDigestOrExactUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyReferenceMatch]{
		newDest: func() json.Unmarshaler { return &prmMatchRepoDigestOrExact{} },
		newValidObject: func() (PolicyReferenceMatch, error) {
			return NewPRMMatchRepoDigestOrExact(), nil
		},
		otherJSONParser: newPolicyReferenceMatchFromJSON,
		invalidObjects: []mSA{
			// Missing "type" field
			{},
			// Wrong "type" field
			{"type": 1},
			{"type": "this is invalid"},
			// Extra fields
			{
				"type":    string(prmTypeMatchRepoDigestOrExact),
				"unknown": "foo",
			},
		},
		duplicateFields: []string{"type"},
	}.run(t)
}

func TestNewPRMMatchRepository(t *testing.T) {
	_prm := NewPRMMatchRepository()
	prm, ok := _prm.(*prmMatchRepository)
	require.True(t, ok)
	assert.Equal(t, &prmMatchRepository{prmCommon{prmTypeMatchRepository}}, prm)
}

func TestPRMMatchRepositoryUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyReferenceMatch]{
		newDest: func() json.Unmarshaler { return &prmMatchRepository{} },
		newValidObject: func() (PolicyReferenceMatch, error) {
			return NewPRMMatchRepository(), nil
		},
		otherJSONParser: newPolicyReferenceMatchFromJSON,
		invalidObjects: []mSA{
			// Missing "type" field
			{},
			// Wrong "type" field
			{"type": 1},
			{"type": "this is invalid"},
			// Extra fields
			{
				"type":    string(prmTypeMatchRepository),
				"unknown": "foo",
			},
		},
		duplicateFields: []string{"type"},
	}.run(t)
}

// xNewPRMExactReference is like NewPRMExactReference, except it must not fail.
func xNewPRMExactReference(dockerReference string) PolicyReferenceMatch {
	pr, err := NewPRMExactReference(dockerReference)
	if err != nil {
		panic("xNewPRMExactReference failed")
	}
	return pr
}

func TestNewPRMExactReference(t *testing.T) {
	const testDR = "library/busybox:latest"

	// Success
	_prm, err := NewPRMExactReference(testDR)
	require.NoError(t, err)
	prm, ok := _prm.(*prmExactReference)
	require.True(t, ok)
	assert.Equal(t, &prmExactReference{
		prmCommon:       prmCommon{prmTypeExactReference},
		DockerReference: testDR,
	}, prm)

	// Invalid dockerReference
	_, err = NewPRMExactReference("")
	assert.Error(t, err)
	// Uppercase is invalid in Docker reference components.
	_, err = NewPRMExactReference("INVALIDUPPERCASE:latest")
	assert.Error(t, err)
	// Missing tag
	_, err = NewPRMExactReference("library/busybox")
	assert.Error(t, err)
}

func TestPRMExactReferenceUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyReferenceMatch]{
		newDest: func() json.Unmarshaler { return &prmExactReference{} },
		newValidObject: func() (PolicyReferenceMatch, error) {
			return NewPRMExactReference("library/busybox:latest")
		},
		otherJSONParser: newPolicyReferenceMatchFromJSON,
		breakFns: []func(mSA){
			// The "type" field is missing
			func(v mSA) { delete(v, "type") },
			// Wrong "type" field
			func(v mSA) { v["type"] = 1 },
			func(v mSA) { v["type"] = "this is invalid" },
			// Extra top-level sub-object
			func(v mSA) { v["unexpected"] = 1 },
			// The "dockerReference" field is missing
			func(v mSA) { delete(v, "dockerReference") },
			// Invalid "dockerReference" field
			func(v mSA) { v["dockerReference"] = 1 },
		},
		duplicateFields: []string{"type", "dockerReference"},
	}.run(t)
}

// xNewPRMExactRepository is like NewPRMExactRepository, except it must not fail.
func xNewPRMExactRepository(dockerRepository string) PolicyReferenceMatch {
	pr, err := NewPRMExactRepository(dockerRepository)
	if err != nil {
		panic("xNewPRMExactRepository failed")
	}
	return pr
}

func TestNewPRMExactRepository(t *testing.T) {
	const testDR = "library/busybox:latest"

	// Success
	_prm, err := NewPRMExactRepository(testDR)
	require.NoError(t, err)
	prm, ok := _prm.(*prmExactRepository)
	require.True(t, ok)
	assert.Equal(t, &prmExactRepository{
		prmCommon:        prmCommon{prmTypeExactRepository},
		DockerRepository: testDR,
	}, prm)

	// Invalid dockerRepository
	_, err = NewPRMExactRepository("")
	assert.Error(t, err)
	// Uppercase is invalid in Docker reference components.
	_, err = NewPRMExactRepository("INVALIDUPPERCASE")
	assert.Error(t, err)
}

func TestPRMExactRepositoryUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyReferenceMatch]{
		newDest: func() json.Unmarshaler { return &prmExactRepository{} },
		newValidObject: func() (PolicyReferenceMatch, error) {
			return NewPRMExactRepository("library/busybox:latest")
		},
		otherJSONParser: newPolicyReferenceMatchFromJSON,
		breakFns: []func(mSA){
			// The "type" field is missing
			func(v mSA) { delete(v, "type") },
			// Wrong "type" field
			func(v mSA) { v["type"] = 1 },
			func(v mSA) { v["type"] = "this is invalid" },
			// Extra top-level sub-object
			func(v mSA) { v["unexpected"] = 1 },
			// The "dockerRepository" field is missing
			func(v mSA) { delete(v, "dockerRepository") },
			// Invalid "dockerRepository" field
			func(v mSA) { v["dockerRepository"] = 1 },
		},
		duplicateFields: []string{"type", "dockerRepository"},
	}.run(t)
}

func TestValidateIdentityRemappingPrefix(t *testing.T) {
	for _, s := range []string{
		"localhost",
		"example.com",
		"example.com:80",
		"example.com/repo",
		"example.com/ns1/ns2/ns3/repo.with.dots-dashes_underscores",
		"example.com:80/ns1/ns2/ns3/repo.with.dots-dashes_underscores",
		// NOTE: These values are invalid, do not actually work, and may be rejected by this function
		// and in NewPRMRemapIdentity in the future.
		"shortname",
		"ns/shortname",
	} {
		err := validateIdentityRemappingPrefix(s)
		assert.NoError(t, err, s)
	}

	for _, s := range []string{
		"",
		"repo_with_underscores", // Not a valid DNS name, at least per docker/reference
		"example.com/",
		"example.com/UPPERCASEISINVALID",
		"example.com/repo/",
		"example.com/repo:tag",
		"example.com/repo@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"example.com/repo:tag@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		err := validateIdentityRemappingPrefix(s)
		assert.Error(t, err, s)
	}
}

// xNewPRMRemapIdentity is like NewPRMRemapIdentity, except it must not fail.
func xNewPRMRemapIdentity(prefix, signedPrefix string) PolicyReferenceMatch {
	pr, err := NewPRMRemapIdentity(prefix, signedPrefix)
	if err != nil {
		panic("xNewPRMRemapIdentity failed")
	}
	return pr
}

func TestNewPRMRemapIdentity(t *testing.T) {
	const testPrefix = "example.com/docker-library"
	const testSignedPrefix = "docker.io/library"

	// Success
	_prm, err := NewPRMRemapIdentity(testPrefix, testSignedPrefix)
	require.NoError(t, err)
	prm, ok := _prm.(*prmRemapIdentity)
	require.True(t, ok)
	assert.Equal(t, &prmRemapIdentity{
		prmCommon:    prmCommon{prmTypeRemapIdentity},
		Prefix:       testPrefix,
		SignedPrefix: testSignedPrefix,
	}, prm)

	// Invalid prefix
	_, err = NewPRMRemapIdentity("", testSignedPrefix)
	assert.Error(t, err)
	_, err = NewPRMRemapIdentity("example.com/UPPERCASEISINVALID", testSignedPrefix)
	assert.Error(t, err)
	// Invalid signedPrefix
	_, err = NewPRMRemapIdentity(testPrefix, "")
	assert.Error(t, err)
	_, err = NewPRMRemapIdentity(testPrefix, "example.com/UPPERCASEISINVALID")
	assert.Error(t, err)
}

func TestPRMRemapIdentityUnmarshalJSON(t *testing.T) {
	policyJSONUmarshallerTests[PolicyReferenceMatch]{
		newDest: func() json.Unmarshaler { return &prmRemapIdentity{} },
		newValidObject: func() (PolicyReferenceMatch, error) {
			return NewPRMRemapIdentity("example.com/docker-library", "docker.io/library")
		},
		otherJSONParser: newPolicyReferenceMatchFromJSON,
		breakFns: []func(mSA){
			// The "type" field is missing
			func(v mSA) { delete(v, "type") },
			// Wrong "type" field
			func(v mSA) { v["type"] = 1 },
			func(v mSA) { v["type"] = "this is invalid" },
			// Extra top-level sub-object
			func(v mSA) { v["unexpected"] = 1 },
			// The "prefix" field is missing
			func(v mSA) { delete(v, "prefix") },
			// Invalid "prefix" field
			func(v mSA) { v["prefix"] = 1 },
			func(v mSA) { v["prefix"] = "this is invalid" },
			// The "signedPrefix" field is missing
			func(v mSA) { delete(v, "signedPrefix") },
			// Invalid "signedPrefix" field
			func(v mSA) { v["signedPrefix"] = 1 },
			func(v mSA) { v["signedPrefix"] = "this is invalid" },
		},
		duplicateFields: []string{"type", "prefix", "signedPrefix"},
	}.run(t)
}
