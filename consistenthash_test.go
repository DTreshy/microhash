package microhash

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	keySize     = 20
	requestSize = 1000
)

const epsilon = 1e-6
const localhostPrefix = "localhost:"

type stringer struct {
	Value int
	Label string
}

// Implementing the String() method for fmt.Stringer
func (m stringer) String() string {
	return fmt.Sprintf("Value: %d, Label: %s", m.Value, m.Label)
}

// CalcEntropy calculates the entropy of m.
func calcEntropy(m map[any]int) float64 {
	if len(m) == 0 || len(m) == 1 {
		return 1
	}

	var (
		entropy float64
		total   int
	)

	for _, v := range m {
		total += v
	}

	for _, v := range m {
		proba := float64(v) / float64(total)
		if proba < epsilon {
			proba = epsilon
		}

		entropy -= proba * math.Log2(proba)
	}

	return entropy / math.Log2(float64(len(m)))
}

func BenchmarkGet(b *testing.B) {
	ch := New()
	for i := 0; i < keySize; i++ {
		ch.Add(localhostPrefix + strconv.Itoa(i))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := 0; i < keySize; i++ {
			ch.Get(i)
		}
	}
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

func BenchmarkAdd(b *testing.B) {
	nodes := make([]string, 20)
	for i := 0; i < keySize; i++ {
		nodes = append(nodes, fmt.Sprintf("localhost:%s", strconv.Itoa(i)))
	}

	for i := 0; i < b.N; i++ {
		ch := New()

		b.ResetTimer()

		for i := 0; i < keySize; i++ {
			ch.Add(nodes[i])
		}
	}
}

func TestConsistentHash(t *testing.T) {
	ch := NewWithCustomHash(0, nil)
	val, ok := ch.Get("any")

	assert.False(t, ok)
	assert.Nil(t, val)

	for i := 0; i < keySize; i++ {
		ch.AddWithReplicas(localhostPrefix+strconv.Itoa(i), minReplicas<<1)
	}

	keys := make(map[string]int)

	for i := 0; i < requestSize; i++ {
		key, ok := ch.Get(requestSize + i)
		assert.True(t, ok)
		keys[key.(string)]++
	}

	mi := make(map[any]int, len(keys))
	for k, v := range keys {
		mi[k] = v
	}

	entropy := calcEntropy(mi)
	assert.True(t, entropy > .95)
}

func TestConsistentHashIncrementalTransfer(t *testing.T) {
	prefix := "anything"
	create := func() *ConsistentHash {
		ch := New()
		for i := 0; i < keySize; i++ {
			ch.Add(prefix + strconv.Itoa(i))
		}

		return ch
	}

	originCh := create()
	keys := make(map[int]string, requestSize)

	for i := 0; i < requestSize; i++ {
		key, ok := originCh.Get(requestSize + i)
		assert.True(t, ok)
		assert.NotNil(t, key)

		keys[i], ok = key.(string)
		assert.True(t, ok)
	}

	node := fmt.Sprintf("%s%d", prefix, keySize)

	for i := 0; i < 10; i++ {
		laterCh := create()
		laterCh.AddWithWeight(node, 10*(i+1))

		for j := 0; j < requestSize; j++ {
			key, ok := laterCh.Get(requestSize + j)
			assert.True(t, ok)
			assert.NotNil(t, key)

			value, ok := key.(string)
			assert.True(t, ok)
			assert.True(t, value == keys[j] || value == node)
		}
	}
}

func TestConsistentHashTransferOnFailure(t *testing.T) {
	index := 41
	keys, newKeys := getKeysBeforeAndAfterFailure(t, localhostPrefix, index)

	var transferred int

	for k, v := range newKeys {
		if v != keys[k] {
			transferred++
		}
	}

	ratio := float32(transferred) / float32(requestSize)
	assert.True(t, ratio < 2.5/float32(keySize), fmt.Sprintf("%d: %f", index, ratio))
}

func TestConsistentHashLeastTransferOnFailure(t *testing.T) {
	prefix := localhostPrefix
	index := 41
	keys, newKeys := getKeysBeforeAndAfterFailure(t, prefix, index)

	for k, v := range keys {
		newV := newKeys[k]
		if v != prefix+strconv.Itoa(index) {
			assert.Equal(t, v, newV)
		}
	}
}

func TestConsistentHash_Remove(t *testing.T) {
	ch := New()
	firstNode := "First"
	secondNode := "Second"

	ch.Add(firstNode)
	ch.Add(secondNode)
	ch.Remove(firstNode)

	for i := 0; i < 100; i++ {
		val, ok := ch.Get(i)
		assert.True(t, ok)
		assert.Equal(t, secondNode, val)
	}

	ch.Remove(secondNode)

	val, ok := ch.Get(true)

	assert.False(t, ok)
	assert.Equal(t, nil, val)
}

func TestConsistentHash_Get(t *testing.T) {
	ch := New()
	node := "Node"

	type testCase struct {
		Name     string
		Value    any
		Expected string
	}

	tests := []testCase{
		{"Get nil", nil, node},
		{"Get int", int(1), node},
		{"Get string", "string", node},
		{"Get bool", true, node},
		{"Get float32", float32(1.2), node},
		{"Get float64", float64(1.22), node},
		{"Get fmt.Stringer", stringer{Value: 5, Label: "fmt.stringer"}, node},
		{"Get int8", int8(1), node},
		{"Get int16", int16(1), node},
		{"Get int32", int32(1), node},
		{"Get int64", int64(1), node},
		{"Get uint", uint(1), node},
		{"Get uint8", uint8(1), node},
		{"Get uint16", uint16(1), node},
		{"Get uint32", uint32(1), node},
		{"Get uint64", uint64(1), node},
		{"Get []byte", []byte("string"), node},
		{"Get error", fmt.Errorf("string"), node},
	}

	// test Get on hash with no nodes
	val, ok := ch.Get(1)
	assert.False(t, ok)
	assert.Equal(t, nil, val)

	ch.Add(node)

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			val, ok := ch.Get(tt.Value)
			assert.True(t, ok)
			assert.Equal(t, tt.Expected, val)
		})
	}
}

func TestConsistentHash_RemoveInterface(t *testing.T) {
	const key = "any"

	ch := New()
	node1 := newMockNode(key, 1)
	node2 := newMockNode(key, 2)

	ch.AddWithWeight(node1, 80)
	ch.AddWithWeight(node2, 50)
	assert.Equal(t, 1, len(ch.nodes))
	node, ok := ch.Get(1)
	assert.True(t, ok)
	assert.Equal(t, key, node.(*mockNode).addr)
	assert.Equal(t, 2, node.(*mockNode).id)
}

func getKeysBeforeAndAfterFailure(t *testing.T, prefix string, index int) (keys, newkeys map[int]string) {
	ch := New()
	for i := 0; i < keySize; i++ {
		ch.Add(prefix + strconv.Itoa(i))
	}

	keys = make(map[int]string, requestSize)

	for i := 0; i < requestSize; i++ {
		key, ok := ch.Get(requestSize + i)
		assert.True(t, ok)
		assert.NotNil(t, key)

		keys[i], ok = key.(string)
		assert.True(t, ok)
	}

	newKeys := make(map[int]string, requestSize)
	remove := fmt.Sprintf("%s%d", prefix, index)

	ch.Remove(remove)

	for i := 0; i < requestSize; i++ {
		key, ok := ch.Get(requestSize + i)
		assert.True(t, ok)
		assert.NotNil(t, key)
		assert.NotEqual(t, remove, key)

		newKeys[i], ok = key.(string)
		assert.True(t, ok)
	}

	return keys, newKeys
}

type mockNode struct {
	addr string
	id   int
}

func newMockNode(addr string, id int) *mockNode {
	return &mockNode{
		addr: addr,
		id:   id,
	}
}

func (n *mockNode) String() string {
	return n.addr
}
