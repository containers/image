package ostree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "ostree", Transport.Name())
}
