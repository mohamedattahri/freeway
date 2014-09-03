package freeway

import (
	"bytes"
	"math"
	"math/big"
	"math/rand"
	"strconv"
	"testing"
)

func TestEquals(t *testing.T) {
	a := NewRandomID()
	if !a.Equals(a) {
		t.Fatal("A randomly generated ID reported not being equal to itself.")
	}
}

func TestLesserThan(t *testing.T) {
	a, b := NewRandomID(), NewRandomID()
	if a.LesserThan(b) == b.LesserThan(a) {
		t.Fatal("Two randomly generated IDs reported being at equi-distant.")
	}
}

func TestGreaterThan(t *testing.T) {
	a, b := NewRandomID(), NewRandomID()
	if a.GreaterThan(b) == b.GreaterThan(a) {
		t.Fatal("Two randomly generated IDs reported being at equi-distant.")
	}
}

func TestDistance(t *testing.T) {
	a, b := NewRandomID(), NewRandomID()
	c := a.Distance(b)
	if !c.Distance(b).Equals(a) || !b.Distance(a).Equals(c) {
		t.Fatal("Two randomly generated IDs reported asymmetric distance values")
	}

	//Bit by bit test
	aInt, bInt, cInt := a.Int(), b.Int(), new(big.Int)
	for i := 0; i < KeySize; i++ {
		cInt.SetBit(cInt, i, aInt.Bit(i)^bInt.Bit(i))
	}
	if computed, _ := NewID(cInt.Bytes()); !computed.Equals(c) {
		t.Fatal("Computing distance bit by bit returned a different result")
	}

	//Byte by byte
	buffer := make([]byte, IDLength)
	for i := 0; i < IDLength; i++ {
		buffer[i] = a[i] ^ b[i]
	}
	if computed, _ := NewID(buffer); !computed.Equals(c) {
		t.Fatal("Computing distance byte by byte returned a different result")
	}
}

func TestString(t *testing.T) {
	a := NewRandomID().String()
	if len([]byte(a)) != IDLength {
		t.Fatal("String representation of the ID returned a value with an incorrect length")
	}
}

func TestInt(t *testing.T) {
	a := NewRandomID()

	b, _ := NewID(a.Int().Bytes())
	if a.Equals(b) == false {
		t.Fatal("Int returned an invalid value", a, b)
	}

	if a.Int().Cmp(b.Int()) != 0 {
		t.Fatal("Int values should be equal")
	}
}

func toBinary(n *big.Int) string {
	var buffer bytes.Buffer
	for i := 0; i < KeySize; i++ {
		buffer.WriteString(strconv.Itoa(int(n.Bit(i))))
	}
	return buffer.String()
}

func TestBucketIndex(t *testing.T) {
	a, b, c, d := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	for i := 0; i < KeySize; i++ {
		a.SetBit(a, i, 0)
		b.SetBit(b, i, 0)
		c.SetBit(c, i, 0)
		d.SetBit(d, i, 0)
	}

	var i, limit int

	root, err := NewID(a.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if index := root.BucketIndex(root, KeySize); index != (KeySize - 1) {
		t.Fatal("Wrong index for root with itself")
	}

	i = 1
	b = b.SetBit(b, i, 1)
	bID, err := NewID(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	limit = KeySize
	if index := bID.BucketIndex(root, limit); index != i {
		t.Fatal("Returns index", index, "while expecting", i)
	}
	b = b.SetBit(b, i, 0)

	i = 89
	b = b.SetBit(b, i, 1)
	limit = 68
	bID, err = NewID(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if index := bID.BucketIndex(root, limit); index != (limit - 1) {
		t.Fatal("Returns index", index, "while expecting", limit-1)
	}
	b.SetBit(b, i, 0)

	// Testing it with simple Log2 calculations
	x, y := big.NewInt(2024), big.NewInt(1000)
	dif := int(math.Log2(float64(new(big.Int).Xor(x, y).Int64())))
	xID, _ := NewID(x.Bytes())
	yID, _ := NewID(y.Bytes())
	if index := xID.BucketIndex(yID, 160); index != dif {
		t.Fatal("Testing BucketIndex with a simple log2 calculation did not return the expected result.", index, "VS", dif)
	}
}

func TestNewID(t *testing.T) {
	a := NewRandomID()
	good, err := NewID([]byte(a.String()))
	if err != nil {
		t.Fatal("Valid data string returned an error")
	}
	if !a.Equals(good) {
		t.Fatal("Parsing the hex string representation of a randomly generated ID returns a different ID")
	}
}

func randomIDArray(length int) IDArray {
	array := make(IDArray, length)
	for i := 0; i < length; i++ {
		array[i] = NewRandomID()
	}
	return array
}

func TestIDArrayLen(t *testing.T) {
	n := rand.Intn(15)
	array := randomIDArray(n)
	if array.Len() != n {
		t.Fatal("Len returned an incorrect length value")
	}
}

func TestIDArraySwap(t *testing.T) {
	array := randomIDArray(2)
	a := array[0]
	b := array[1]
	array.Swap(0, 1)
	if !array[0].Equals(b) || !array[1].Equals(a) {
		t.Fatal("Value swapping did not work as expected")
	}
}

func TestIDArrayLess(t *testing.T) {
	array := randomIDArray(2)
	if array.Less(0, 1) != array[0].LesserThan(array[1]) {
		t.Fatal("Less testing did not work as expected")
	}
}

func TestIDArraySort(t *testing.T) {
	array := randomIDArray(10)
	array.Sort()
	for i := 1; i < array.Len(); i++ {
		if !array[i].GreaterThan(array[i-1]) {
			t.Fatal("Sorting did not work as expected")
		}
	}
}
