package hash

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

func BenchmarkConsistentHashGet(b *testing.B) {
	ch := NewConsistentHash()
	for i := 0; i < keySize; i++ {
		ch.Add(localhostPrefix + strconv.Itoa(i))
	}

	for i := 0; i < b.N; i++ {
		ch.Get(i)
	}
}

func TestConsistentHash(t *testing.T) {
	ch := NewCustomConsistentHash(0, nil)
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
		ch := NewConsistentHash()
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
	ch := NewConsistentHash()

	ch.Add("first")
	ch.Add("second")
	ch.Remove("first")

	for i := 0; i < 100; i++ {
		val, ok := ch.Get(i)
		assert.True(t, ok)
		assert.Equal(t, "second", val)
	}
}

func TestConsistentHash_RemoveInterface(t *testing.T) {
	const key = "any"

	ch := NewConsistentHash()
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
	ch := NewConsistentHash()
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
