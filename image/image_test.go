package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUniqueLayerDigests(t *testing.T) {
	for _, test := range []struct{ input, expected []string }{
		// Ensure that every element of expected: is unique!
		{input: []string{}, expected: []string{}},
		{input: []string{"a"}, expected: []string{"a"}},
		{input: []string{"a", "b", "c"}, expected: []string{"a", "b", "c"}},
		{input: []string{"a", "a", "c"}, expected: []string{"a", "c"}},
		{input: []string{"a", "b", "a"}, expected: []string{"a", "b"}},
	} {
		in := []fsLayersSchema1{}
		for _, e := range test.input {
			in = append(in, fsLayersSchema1{e})
		}

		m := manifestSchema1{FSLayers: in}
		res := uniqueLayerDigests(&m)
		// Test that the length is the same and each expected element is present.
		// This requires each element of test.expected to be unique, as noted above.
		assert.Len(t, res, len(test.expected))
		for _, e := range test.expected {
			assert.Contains(t, res, e)
		}
	}
}
