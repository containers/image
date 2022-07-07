package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalidSignatureError(t *testing.T) {
	// A stupid test just to keep code coverage
	s := "test"
	err := NewInvalidSignatureError(s)
	assert.Equal(t, s, err.Error())
}
