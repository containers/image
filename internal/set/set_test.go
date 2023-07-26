package set

import (
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
	assert.ElementsMatch(t, []int{1, 2}, s.Values())
	// Unrelated elements are unaffected
	assert.True(t, s.Contains(1))
	assert.False(t, s.Contains(3))
}

func TestAddSlice(t *testing.T) {
	s := NewWithValues(1)
	s.Add(2)
	s.AddSlice([]int{3, 4})
	assert.ElementsMatch(t, []int{1, 2, 3, 4}, s.Values())
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

func TestValues(t *testing.T) {
	s := New[int]()
	assert.Empty(t, s.Values())
	s.Add(1)
	s.Add(2)
	// ignore duplicate
	s.Add(2)
	assert.ElementsMatch(t, []int{1, 2}, s.Values())
}
