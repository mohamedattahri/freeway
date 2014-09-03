package freeway

import (
	"sort"
)

// Represents a peer item stored in the Tree Map.
type PeerState struct {
	queried  bool // Value indicating whether the peer has been queried during this FIND_NODE round.
	replied  bool // Value indicating whether the peer has replied during this FIND_NODE round.
	distance ID   // Distance value is cached as it is extensively used for sorting.
	peer     *Peer
}

// A data structure used to keep track of the peers discovered during an ID lookup.
// Peers discovered are stored and sorted by distance from a given key.
// PeersCloserThan and UnqueriedPeers are there to feed different rounds of a FIND_NODE-RPC
// with peers to query.
type TreeMap struct {
	key     ID
	peers   []*Peer
	hashMap map[string]*PeerState
}

func (tree *TreeMap) contains(peer *Peer) bool {
	_, exists := tree.hashMap[peer.ID().String()]
	return exists
}

func (tree *TreeMap) Len() int {
	return len(tree.peers)
}

func (tree *TreeMap) Swap(a, b int) {
	tree.peers[a], tree.peers[b] = tree.peers[b], tree.peers[a]
}

func (tree *TreeMap) Less(a, b int) bool {
	return tree.peers[a].ID().Distance(tree.key).LesserThan(tree.peers[b].ID().Distance(tree.key))
}

func (tree *TreeMap) Sort() {
	sort.Sort(tree)
}

// Adds a peer to the tree before sorting items.
func (tree *TreeMap) add(peer *Peer) bool {
	return tree.addRange([]*Peer{peer}) == 1
}

// Adds a set a of peers to the tree before applying a sort.
func (tree *TreeMap) addRange(peers []*Peer) (added int) {
	for _, peer := range peers {
		if !tree.contains(peer) {
			tree.hashMap[peer.ID().String()] = &PeerState{distance: peer.ID().Distance(tree.key), peer: peer}
			tree.peers = append(tree.peers, peer)
			added++
		}
	}
	tree.Sort()
	return added
}

// Returns the Alpha unqueried peers in the tree which are closer to the key than a given peer.
func (tree *TreeMap) peersCloserThan(peer *Peer) []*Peer {
	// This should be pretty efficient because the peers array is always sorted by distance from the key.
	closer := make([]*Peer, 0)
	for i, d := 0, peer.ID().Distance(tree.key); i < len(tree.peers) && len(closer) < Alpha; i++ {
		if tree.hashMap[tree.peers[i].ID().String()].distance.LesserThan(d) && tree.hashMap[tree.peers[i].ID().String()].queried == false {
			closer = append(closer, tree.peers[i])
		}
	}
	return closer
}

// Returns the current state of a peer stored in the tree.
func (tree *TreeMap) state(peer *Peer) *PeerState {
	value, _ := tree.hashMap[peer.ID().String()]
	return value
}

// Returns the sorted list of peers stored in the tree.
func (tree *TreeMap) Peers() []*Peer {
	return tree.peers
}

// Returns the K unqueried peers
func (tree *TreeMap) unqueriedPeers() []*Peer {
	peers := make([]*Peer, 0)
	for i := 0; i < len(tree.peers) && len(peers) < K; i++ {
		if tree.hashMap[tree.peers[i].ID().String()].queried == false {
			peers = append(peers, tree.peers[i])
		}
	}
	return peers
}

func NewTreeMap(key ID) *TreeMap {
	return &TreeMap{
		key:     key,
		peers:   make([]*Peer, 0),
		hashMap: make(map[string]*PeerState),
	}
}
