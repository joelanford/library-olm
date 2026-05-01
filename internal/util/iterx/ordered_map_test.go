package iterx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortedMap(t *testing.T) {
	m := map[string]int{"cherry": 3, "apple": 1, "banana": 2}

	var keys []string
	var values []int
	for k, v := range SortedMap(m) {
		keys = append(keys, k)
		values = append(values, v)
	}
	assert.Equal(t, []string{"apple", "banana", "cherry"}, keys)
	assert.Equal(t, []int{1, 2, 3}, values)
}

func TestSortedMap_Empty(t *testing.T) {
	var count int
	for range SortedMap(map[string]int{}) {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestSortedMap_EarlyBreak(t *testing.T) {
	m := map[int]string{3: "c", 1: "a", 2: "b"}

	var keys []int
	for k := range SortedMap(m) {
		keys = append(keys, k)
		if k == 2 {
			break
		}
	}
	assert.Equal(t, []int{1, 2}, keys)
}

func TestSortedMap_IntKeys(t *testing.T) {
	m := map[int]string{30: "thirty", 10: "ten", 20: "twenty"}

	var keys []int
	for k := range SortedMap(m) {
		keys = append(keys, k)
	}
	assert.Equal(t, []int{10, 20, 30}, keys)
}
