package freeway

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/big"
	"math/rand"
	"sort"
	"time"
)

const (
	// IDLength in bytes
	IDLength int = (KeySize / 8)
)

// ID represents a node ID.
type ID [IDLength]byte

// Equals compares two IDs and returns a value indicating whether
// they are equal or not.
func (id ID) Equals(other ID) bool {
	return bytes.Equal(id[:], other[:])
}

// LesserThan compares two IDs and returns a value indicating whether an ID is lesser
// than another.
func (id ID) LesserThan(other ID) bool {
	return bytes.Compare(id[:], other[:]) == -1
}

// GreaterThan compares two IDs and returns a value indicating whether an ID is greater
// than another
func (id ID) GreaterThan(other ID) bool {
	return !id.LesserThan(other)
}

// Distance returns the distance between two nodes
func (id ID) Distance(other ID) (result ID) {
	distance := new(big.Int)
	distance = distance.Xor(id.Int(), other.Int())
	result, _ = NewID(distance.Bytes())
	return
}

// Int converts the byte array to an integer
func (id ID) Int() *big.Int {
	value := new(big.Int)
	value.SetBytes(id.Bytes())
	return value
}

// Bytes return a byte array representation of the ID.
func (id ID) Bytes() []byte {
	return id[:]
}

// String returns a string with the hex representation of the ID
func (id ID) String() string {
	return string(id[:])
}

// Hex returns a hex string representation of the ID
func (id ID) Hex() string {
	return hex.EncodeToString(id[:])
}

// BucketIndex returns the index of the kBucket where the ID is located or should be inserted.
// It's determined using the formula index = Log2(distance) where d is the distance
func (id ID) BucketIndex(root ID, limit int) int {
	distance := id.Distance(root).Int()
	for i := 0; i < limit; i++ {
		if b := distance.Bit(i); b != 0 {
			return i
		}
	}
	// IDs are equal.
	return limit - 1
}

// NewID creates and returns a Node ID from an hex representation of a ID.
func NewID(data []byte) (id ID, err error) {
	if len(data) > IDLength {
		return id, errors.New("byte array's length should inferior to the KeySize const")
	}
	copy(id[IDLength-len(data):], data[:])
	return id, err
}

// NewRandomID generates and returns a random Node ID
func NewRandomID() (id ID) {
	for i := 0; i < IDLength; i++ {
		id[i] = uint8(rand.Intn(256))
	}
	return
}

// IDArray is a simple type aliasing to refer to an array of IDs.
type IDArray []ID

// Contains returns true if the id is in the array.
func (items IDArray) Contains(id ID) bool {
	for i := 0; i < len(items); i++ {
		if items[i].Equals(id) {
			return true
		}
	}
	return false
}

func (items IDArray) Len() int {
	return len(items)
}

func (items IDArray) Swap(a, b int) {
	items[a], items[b] = items[b], items[a]
}

func (items IDArray) Less(a, b int) bool {
	return items[a].LesserThan(items[b])
}

// Sort the array by ID value
func (items IDArray) Sort() {
	sort.Sort(items)
}

// IMPORTANT:
// Necessary to effectively generate random numbers.
func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}
