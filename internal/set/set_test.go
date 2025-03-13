package set

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	s := New[int]()
	assert.True(t, s.Empty())
}

func TestNewWithValues(t *testing.T) {
	s := NewWithValues(1, 3)
	assert.True(t, s.Contains(1))
	assert.False(t, s.Contains(2))
	assert.True(t, s.Contains(3))
}

func TestAdd(t *testing.T) {
	s := NewWithValues(1)
	assert.False(t, s.Contains(2))
	s.Add(2)
	assert.True(t, s.Contains(2))
	s.Add(2) // Adding an already-present element
	assert.True(t, s.Contains(2))
	// should not contain duplicate value of `2`
	assert.ElementsMatch(t, []int{1, 2}, slices.Collect(s.All()))
	// Unrelated elements are unaffected
	assert.True(t, s.Contains(1))
	assert.False(t, s.Contains(3))
}

func TestAddSeq(t *testing.T) {
	s := NewWithValues(1)
	s.Add(2)
	s.AddSeq(slices.Values([]int{3, 4}))
	assert.ElementsMatch(t, []int{1, 2, 3, 4}, slices.Collect(s.All()))
}

func TestDelete(t *testing.T) {
	s := NewWithValues(1, 2)
	assert.True(t, s.Contains(2))
	s.Delete(2)
	assert.False(t, s.Contains(2))
	s.Delete(2) // Deleting a missing element
	assert.False(t, s.Contains(2))
	// Unrelated elements are unaffected
	assert.True(t, s.Contains(1))
}

func TestContains(t *testing.T) {
	s := NewWithValues(1, 2)
	assert.True(t, s.Contains(1))
	assert.True(t, s.Contains(2))
	assert.False(t, s.Contains(3))
}

func TestEmpty(t *testing.T) {
	s := New[int]()
	assert.True(t, s.Empty())
	s.Add(1)
	assert.False(t, s.Empty())
	s.Delete(1)
	assert.True(t, s.Empty())
}

func TestAll(t *testing.T) {
	s := New[int]()
	assert.Empty(t, slices.Collect(s.All()))
	s.Add(1)
	s.Add(2)
	// ignore duplicate
	s.Add(2)
	assert.ElementsMatch(t, []int{1, 2}, slices.Collect(s.All()))
	// Break / return inside the range body (yield function returning false) works
	var partial []int
	for v := range s.All() {
		partial = append(partial, v)
		break
	}
	assert.Len(t, partial, 1)
	assert.True(t, partial[0] == 1 || partial[0] == 2)
}
