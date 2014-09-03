package freeway

/*
	When a Kademlia node received any message (request or reply) from another
	node, it updates the appropriate k-bucket for the sender's node ID. If the
	sending node already exists in the recipient's k-bucket, the recipient moves
	it to the tail of the list. If the node is not already in the appropriate
	k-bucket and the bucket has fewer than k entries, then the recipient just
	inserts the new sender at the tail of the list. If the appropriate k-bucket
	is full, however, then the recipient pings the k-bucket's least recently
	seen node to decide what to do. If the least-recently seen node fails to
	respond, it is evicted from the k-bucket and the new sender is inserted at
	the tail of the list, and the new sender's contact is discarded.
*/

import (
	"container/list"
	"errors"
	"math"
	"time"
)

const (
	// maintainanceFrequency indicates how often the buckets are maintained and refreshed
	// following the Kademlia specifications.
	maintainanceFrequency = 1 * time.Hour
)

// ErrBucketMustSplit is the error returned by a bucket when it's full and needs a split.
var ErrBucketMustSplit = errors.New("bucket is full and must be split")

// A Bucket is a collection of nodes grouped by their distance from the local peer's ID.
// The index int here is a reference to the precision-bit at which the nodes it contains are
// considered as "close" to the local peer.
type Bucket struct {
	set        *BucketSet
	index      int
	splittable bool
	peers      *list.List
}

// Peers simply returns the list of peers stored in the current bucket
func (bucket *Bucket) Peers() []*Peer {
	peers := make([]*Peer, bucket.peers.Len())
	i := 0
	for e := bucket.peers.Front(); e != nil; e = e.Next() {
		peers[i] = e.Value.(*Peer)
		i++
	}
	array := PeerArray(peers[:])
	array.Sort()
	return array
}

// PeerSlice returns a slice of the nodes contained in the bucket with no risk of index over-flow.
func (bucket *Bucket) PeerSlice(from, length int) []*Peer {
	if bucket.Len() == 0 {
		return []*Peer{}
	}
	length = int(math.Min(float64(length), float64(bucket.Len()-from)))
	return bucket.Peers()[from:length]
}

// Len returns the number of peers currently in the bucket
func (bucket *Bucket) Len() int {
	return bucket.peers.Len()
}

// removeElement removes an element from the bucket, and signals to the BucketSet that the peer
// has been removed.
func (bucket *Bucket) removeElement(e *list.Element) {
	bucket.peers.Remove(e)
	bucket.set.unreference(e.Value.(*Peer))
}

// Split is called when the bucket is full and needs to be split following the
// binary tree requirements set by the Kademlia research paper.
// It returns an array containing the Peer instances which are considered as "far"
// and should therefore be added to a new bucket.
func (bucket *Bucket) split() (removed []*Peer) {
	bucket.splittable = false

	limit := bucket.index + 2 // +1 (current length) +1 (target length).
	for e := bucket.peers.Front(); e != nil; e = e.Next() {
		if e.Value.(*Peer).BucketIndex(limit) > bucket.index {
			removed = append(removed, e.Value.(*Peer))
			defer bucket.removeElement(e)
		}
	}
	return
}

// freeUpSpaceFor is responsible for keeping the bucket full of reliable and healthy peers.
// When a bucket is full, the bucket must decide whether to reject new candidates, or accept them to replace
// known peers with a bad health record. Some peers are given a second chance with a ping request,
// and other are directly removed by fresher peers.
func (bucket *Bucket) freeUpSpaceFor(candidate *Peer) {
	if bucket.Len() < K {
		bucket.insert(candidate)
		return
	}

	for e := bucket.peers.Front(); e != nil; e = e.Next() {
		peer := e.Value.(*Peer)
		alive, refresh := peer.IsAlive()
		if alive {
			continue
		}

		// Peer is considered totally dead.
		if !refresh {
			bucket.removeElement(e)
			bucket.insert(candidate)
			break
		}

		// Peer will be pinged and the item added if a response is returned.
		go func() {
			peer.Ping()
			time.Sleep(Timeout)

			bucket.insert(candidate)
		}()
		break
	}
}

// Add attempts to add a Peer to the current bucket.
// If the peer is already in, it will simply be moved to the back.
// Otherwise, the peer will either be added to the front, or the
// Bucket will return a ErrBucketMustSplit to notify the parent BucketSet
// that the Split() method should be called.
// If the bucket is full and has already been split, the bucket will attempt
// to ping the least recent (front) peers it knows.
func (bucket *Bucket) insert(candidate *Peer) (err error) {
	//If the candidate is in the bucket, move it to the back.
	for e := bucket.peers.Front(); e != nil; e = e.Next() {
		if candidate.ID().Equals(e.Value.(*Peer).ID()) {
			defer bucket.peers.MoveToBack(e)
			err = errors.New("peer already exists")
			return
		}
	}

	//If the candidate is not in the bucket, and the bucket has not reached its
	//full capacity, add the candidate to the front.
	if bucket.peers.Len() < K {
		bucket.peers.PushFront(candidate)
		return
	}

	// Bucket is full, must split
	if bucket.splittable {
		return ErrBucketMustSplit
	}

	// Bucket is full. Peer is likely to be ignored.
	// Must check the health of the peers it knows.
	bucket.freeUpSpaceFor(candidate)
	return nil
}

// AddArray is a handy method to call the Insert method with an array of peers instead.
func (bucket *Bucket) insertRange(peers []*Peer) {
	for i := 0; i < len(peers); i++ {
		bucket.insert(peers[i])
	}
}

// If in the range of a k-bucket no lookup has been performed for an hour,
// this k-bucket must be refreshed.
// This means that a node lookup is performed for an id that falls in the range of this k-bucket.
// needsLockup returns a value indicating whether its time for the bucket to be refreshed.
func (bucket *Bucket) needsLookup() bool {
	now := time.Now()
	for e := bucket.peers.Front(); e != nil; e = e.Next() {
		if now.Sub(e.Value.(*Peer).lastLookup) < maintainanceFrequency {
			return false
		}
	}
	return true
}

// startMaintainer launches the maintainance go-routine that's fired every hour.
func (bucket *Bucket) startMaintainer() {
	for {
		time.Sleep(maintainanceFrequency)
		if bucket.needsLookup() {
			id := bucket.peers.Front().Value.(*Peer).ID()
			bucket.set.dht.FindNode(id, nil)
		}
	}
}

// NewBucket returns an instance of the Bucket struct and ties it to the
// parent bucket set. The index int refers to the Kademlia kBucket index that will
// assigned to the newly created instance.
func NewBucket(set *BucketSet, index int) (instance *Bucket) {
	splittable := index < KeySize
	instance = &Bucket{
		set:        set,
		index:      index,
		splittable: splittable, //The last kBucket is obviously not splittable.
		peers:      list.New(),
	}
	go instance.startMaintainer()
	return
}

// A BucketSet is the basis of the Kademlia routing table.
// It represents a binary tree of kBuckets structured to cover the entire range of the KeySize space.
type BucketSet struct {
	dht     *DHT
	buckets []*Bucket
	hashMap map[string]*Peer
}

// Len returns the number of peers stored in the bucket set.
func (set *BucketSet) Len() (length int) {
	for i := 0; i < len(set.buckets); i++ {
		length += set.buckets[i].Len()
	}
	return
}

// Contains returns a value indicating whether the bucket set contains a peer with a given HEX value.
func (set *BucketSet) Contains(id ID) (exists bool) {
	_, exists = set.hashMap[id.String()]
	return
}

// Peer returns a peer stored in the bucket set using its HEX value.
func (set *BucketSet) Peer(id ID) *Peer {
	peer, _ := set.hashMap[id.String()]
	return peer
}

// Technically, peers are stored in bucket and managed by the bucket set.
// Unreference removes all references to a peer after it has been deleted from the bucket.
func (set *BucketSet) unreference(peer *Peer) {
	delete(set.hashMap, peer.ID().String())
}

// Insert attempts to insert a Peer instance in the right kBucket.
// If the Bucket is full and requests splitting, the method will proceed and
// and make another attempt.
func (set *BucketSet) Insert(peer *Peer) {
	index := peer.BucketIndex(len(set.buckets))

	// The BucketSet is totally empty
	if len(set.buckets) == 0 {
		set.addBucketWithPeers([]*Peer{peer})
		return
	}

	// Call Bucket.Add, and watch for ErrBucketMustSplit in case splitting
	// is going to be necessary.
	if err := set.buckets[index].insert(peer); err == nil {
		//Item was successfully added. Should be added to the map too.
		set.hashMap[peer.ID().String()] = peer
	} else if err == ErrBucketMustSplit && index < KeySize {
		set.addBucketWithPeers(set.buckets[index].split())
		set.Insert(peer)
	}
}

// InsertRange is a handy method to insert more than one peer a time.
func (set *BucketSet) InsertRange(peers []*Peer) {
	for _, peer := range peers {
		set.Insert(peer)
	}
}

// addBucketWithPeers attempts to create a new Bucket following a split,
// and fills it with the peers removed from the previous bucket.
func (set *BucketSet) addBucketWithPeers(peers []*Peer) {
	// 1 bucket per bit is the maximum
	if len(set.buckets) >= KeySize {
		return
	}
	newBucket := NewBucket(set, len(set.buckets))
	set.buckets = append(set.buckets, newBucket)
	newBucket.insertRange(peers)
}

// ClosestPeersFrom returns the Alpha peers which are the closest to a given key
func (set *BucketSet) ClosestPeersFrom(key ID) (nodes []*Peer) {
	// Protection against out of range indexing
	index := key.BucketIndex(set.dht.LocalPeer().ID(), len(set.buckets))

	for i := index; i > 0 && len(nodes) < Alpha; i-- {
		nodes = append(nodes, set.buckets[i].PeerSlice(0, Alpha-len(nodes))...)
	}

	for i := index; i < len(set.buckets) && len(nodes) < Alpha; i++ {
		nodes = append(nodes, set.buckets[i].PeerSlice(0, Alpha-len(nodes))...)
	}

	return
}

// NewBucketSet returns a new instance of the BucketSet struct, and ties it to a DHT instance.
// The local Peer will automatically be inserted.
func NewBucketSet(dht *DHT) (instance *BucketSet) {
	instance = &BucketSet{
		dht:     dht,
		buckets: make([]*Bucket, 0),
		hashMap: make(map[string]*Peer),
	}
	return
}
