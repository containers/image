package multierr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type errA struct{}

func (errA) Error() string { return "A" }

func TestFormat(t *testing.T) {
	errB := errors.New("B")

	// Single-item Format preserves the error type
	res := Format("[", ",", "]", []error{errA{}})
	assert.Equal(t, "A", res.Error())
	var aTarget errA
	assert.ErrorAs(t, res, &aTarget)

	// Single-item Format preserves the error identity
	res = Format("[", ",", "]", []error{errB})
	assert.Equal(t, "B", res.Error())
	assert.ErrorIs(t, res, errB)

	// Multi-item Format preserves both
	res = Format("[", ",", "]", []error{errA{}, errB})
	assert.Equal(t, "[A,B]", res.Error())
	assert.ErrorAs(t, res, &aTarget)
	assert.ErrorIs(t, res, errB)

	// This is invalid, but make sure we don’t misleadingly suceeed
	res = Format("[", ",", "]", []error{})
	assert.Error(t, res)
	res = Format("[", ",", "]", nil)
	assert.Error(t, res)
}
