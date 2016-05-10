package signature

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mSI map[string]interface{} // To minimize typing the long name

// A short-hand way to get a JSON object field value or panic. No error handling done, we know
// what we are working with, a panic in a test is good enough, and fitting test cases on a single line
// is a priority.
func x(m mSI, fields ...string) mSI {
	for _, field := range fields {
		// Not .(mSI) because type assertion of an unnamed type to a named type always fails (the types
		// are not "identical"), but the assignment is fine because they are "assignable".
		m = m[field].(map[string]interface{})
	}
	return m
}

func TestValidateExactMapKeys(t *testing.T) {
	// Empty map and keys
	err := validateExactMapKeys(mSI{})
	assert.NoError(t, err)

	// Success
	err = validateExactMapKeys(mSI{"a": nil, "b": 1}, "b", "a")
	assert.NoError(t, err)

	// Extra map keys
	err = validateExactMapKeys(mSI{"a": nil, "b": 1}, "a")
	assert.Error(t, err)

	// Extra expected keys
	err = validateExactMapKeys(mSI{"a": 1}, "b", "a")
	assert.Error(t, err)

	// Unexpected key values
	err = validateExactMapKeys(mSI{"a": 1}, "b")
	assert.Error(t, err)
}

func TestMapField(t *testing.T) {
	// Field not found
	_, err := mapField(mSI{"a": mSI{}}, "b")
	assert.Error(t, err)

	// Field has a wrong type
	_, err = mapField(mSI{"a": 1}, "a")
	assert.Error(t, err)

	// Success
	// FIXME? We can't use mSI as the type of child, that type apparently can't be converted to the raw map type.
	child := map[string]interface{}{"b": mSI{}}
	m, err := mapField(mSI{"a": child, "b": nil}, "a")
	require.NoError(t, err)
	assert.Equal(t, child, m)
}

func TestStringField(t *testing.T) {
	// Field not found
	_, err := stringField(mSI{"a": "x"}, "b")
	assert.Error(t, err)

	// Field has a wrong type
	_, err = stringField(mSI{"a": 1}, "a")
	assert.Error(t, err)

	// Success
	s, err := stringField(mSI{"a": "x", "b": nil}, "a")
	require.NoError(t, err)
	assert.Equal(t, "x", s)
}
