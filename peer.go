package freeway

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"time"
)

const (
	CompactNodeInfoLength = IDLength + 6
)

// A peer is a contact on the network.
type Peer struct {
	dht         *DHT
	addr        *net.UDPAddr
	id          ID
	hex         string
	info        []byte
	pinging     bool
	lastQueried time.Time
	lastReplied time.Time
	lastLookup  time.Time
}

// DHT to which this Peer is attached
func (peer *Peer) DHT() *DHT {
	return peer.dht
}

// Host on which the peer can be contacted.
func (peer *Peer) Addr() *net.UDPAddr {
	return peer.addr
}

// ID of the peer
func (peer *Peer) ID() ID {
	return peer.id
}

// Returns a unique HEX value for this peer.
// Most Kademlia implementation will only hash the ID.
// We chose to include the host and the port to allow multiple nodes
// to live on the machine.
func (peer *Peer) Hex() string {
	return peer.hex
}

// Returns a CompactNodeInfo representation
func (peer *Peer) CompactNodeInfo() []byte {
	return peer.info
}

func (peer *Peer) isLocal() bool {
	return peer.dht.LocalPeer().Equals(peer)
}

func (peer *Peer) Equals(other *Peer) bool {
	return peer.addr.IP.Equal(other.addr.IP) && peer.addr.Port == other.addr.Port && peer.id.Equals(other.id)
}

// Evaluates the health status of a peer.
// IsAlive returns two boolean.
// The first one indicates whether the peer is considered active or not.
// The second indicates whether the peer is worth to be pinged.
func (peer *Peer) IsAlive() (bool, bool) {
	now := time.Now()

	// Peer is brand new.
	if peer.lastQueried == peer.lastReplied {
		return true, false
	}

	// Last reply is pretty recent.
	if now.Sub(peer.lastReplied) < Freshness {
		return true, false
	}

	// At this point, we know the latest reply is pretty old.
	// We should determine whether the node has not been queried for a while,
	// or if it's actually not replying.
	if now.Sub(peer.lastQueried) > Freshness {
		// Peer has not been queried for a while.
		// A Ping would be totally appropriate.
		return false, true
	}

	// Ping has not replied in a while.
	return false, false
}

// Updates the info and health status of the peer following reply.
func (peer *Peer) updateAfterReply(response *IncomingReplyPacket) {
	peer.lastReplied = time.Now()
}

// Send a ping RPC request to the current Peer
func (peer *Peer) Ping() {
	if peer.isLocal() || peer.pinging {
		return
	}

	responseChan := make(chan *IncomingReplyPacket, 1)
	go func() {
		select {
		case response := <-responseChan:
			peer.updateAfterReply(response)
		case <-time.After(Timeout):
		}
		peer.pinging = false
	}()

	query := NewPingQuery(peer.ID())
	req := NewRequest(peer.dht, peer, query, responseChan)
	peer.dht.krpc.Send(req)
	peer.pinging = true
	peer.lastQueried = time.Now()
}

// Extracts concatenated nodes in Compact Node Info format from a byte array.
func (peer *Peer) parsePeersFromResponse(data []byte) []*Peer {
	if (len(data) % CompactNodeInfoLength) != 0 {
		panic("Data is misformatted.") //Not sure panic is advised here...
	}

	peers := make([]*Peer, len(data)/CompactNodeInfoLength)
	for i := 0; i < len(peers); i++ {
		peer, err := NewPeerFromCompactNodeInfo(peer.dht, data[i*CompactNodeInfoLength:CompactNodeInfoLength*(i+1)])
		if err != nil {
			log.Println("Could not parse Compact info for a node. Skipping index", i)
			continue
		}
		peers[i] = peer
	}
	return peers
}

// Sends a find_node RPC request to the current Peer
func (peer *Peer) FindNode(key ID, closestPeers chan *FindNodeResponse) {
	fail := func() {
		//TODO: return nil instead of an empty array.
		closestPeers <- &FindNodeResponse{peer, []*Peer{}}
	}

	if peer.isLocal() == true {
		fail()
		return
	}

	responseChan := make(chan *IncomingReplyPacket)
	go func() {
		select {
		case response := <-responseChan:
			peer.updateAfterReply(response)
			raw, valid := response.ReplyPacket.NamedReturns["nodes"].(string)
			if !valid {
				log.Println("Received data in invalid", raw)
				fail()
				return
			}
			closestPeers <- &FindNodeResponse{peer, peer.parsePeersFromResponse([]byte(raw))}
		case <-time.After(Timeout):
			fail()
			return
		}
	}()

	query := NewFindNodeQuery(peer.dht.LocalPeer().ID(), key)
	req := NewRequest(peer.dht, peer, query, responseChan)
	peer.dht.krpc.Send(req)
	peer.lastLookup = time.Now()
	peer.lastQueried = time.Now()
}

// BucketIndex returns the index of the kBucket where the ID is located or should be inserted.
// It's determined using the formula i = Log2(d) where d is the distance and i is the index we're looking for.
func (peer *Peer) BucketIndex(limit int) int {
	return peer.ID().BucketIndex(peer.dht.LocalPeer().ID(), limit)
}

// The distance represented as an ID between this Peer and another.
func (peer *Peer) DistanceFrom(other *Peer) ID {
	return peer.id.Distance(other.id)
}

func (peer *Peer) DistanceFromRoot() ID {
	return peer.DistanceFrom(peer.DHT().LocalPeer())
}

func (peer *Peer) String() string {
	return fmt.Sprintf("[%s:%s]", peer.id.Hex(), peer.addr.String())
}

func NewPeer(dht *DHT, addr *net.UDPAddr, id ID) *Peer {
	info := PeerCompactNodeInfo(addr, id)
	hex := hex.EncodeToString(info)

	// If the peer is known, return the cached version instead.
	cached := dht.buckets.Peer(id)
	if cached != nil {
		return cached
	}

	return &Peer{
		dht:  dht,
		addr: addr,
		id:   id,
		hex:  hex,
		info: info,
	}
}

// NewPeerFromAddr creates a new peer from a UDP address
func NewPeerFromAddr(dht *DHT, addr *net.UDPAddr, id ID) (*Peer, error) {
	return NewPeer(dht, addr, id), nil
}

// NewPeerFromIP creates a new peer from an ip and port
func NewPeerFromIP(dht *DHT, ip string, port int, id ID) (*Peer, error) {
	rAddr, err := net.ResolveUDPAddr(ipv, net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	return NewPeer(dht, rAddr, id), nil
}

// NewPeerFromCompactAddressPort creates a new peer from a KRPC compact address format.
// Contact information for peers is encoded as a 6-byte string.
// Also known as "Compact IP-address/port info" the 4-byte IP address is in network byte order
// with the 2 byte port in network byte order concatenated onto the end.
func NewPeerFromCompactAddressPort(dht *DHT, info []byte) (*Peer, error) {
	if len(info) != 6 {
		return nil, errors.New("Misformatted Compact IP-address/port info")
	}
	ip := net.IPv4(info[0], info[1], info[2], info[3]).String()
	port := int((uint16(info[4]) << 8) + uint16(info[5]))
	peer, err := NewPeerFromIP(dht, ip, int(port), NewRandomID())
	if err != nil {
		return nil, err
	}
	return peer, nil
}

// Contact information for nodes is encoded as a 26-byte string.
// Also known as "Compact node info" the 20-byte Node ID in network byte order
// has the compact IP-address/port info concatenated to the end.
func NewPeerFromCompactNodeInfo(dht *DHT, info []byte) (*Peer, error) {
	if len(info) != CompactNodeInfoLength {
		return nil, errors.New("Misformatted Compact node info")
	}
	id, _ := NewID(info[0:IDLength])
	ip := net.IPv4(info[IDLength], info[IDLength+1], info[IDLength+2], info[IDLength+3]).String()
	port := (uint16(info[IDLength+4]) << 8) + uint16(info[IDLength+5])

	// Build up a new Peer with the parsed info.
	peer, err := NewPeerFromIP(dht, ip, int(port), id)
	if err != nil {
		return nil, err
	}
	return peer, nil
}

// Simple type aliasing to represent an array of Peers
type PeerArray []*Peer

// Returns true if the id is in the array.
func (peers PeerArray) Contains(peer *Peer) bool {
	for i := 0; i < len(peers); i++ {
		if peers[i].ID().Equals(peer.ID()) {
			return true
		}
	}
	return false
}

func (peers PeerArray) Len() int {
	return len(peers)
}

func (peers PeerArray) Swap(a, b int) {
	peers[a], peers[b] = peers[b], peers[a]
}

func (peers PeerArray) Less(a, b int) bool {
	return peers[a].DistanceFromRoot().LesserThan(peers[b].DistanceFromRoot())
}

func (peers PeerArray) Sort() {
	sort.Sort(peers)
}

func (peers PeerArray) CompactInfo() []byte {
	info := make([]byte, 0)
	for _, peer := range peers {
		info = append(info, peer.CompactNodeInfo()...)
	}
	return info
}

func PeerCompactNodeInfo(addr *net.UDPAddr, id ID) []byte {
	info := make([]byte, CompactNodeInfoLength)
	copy(info[0:], id[:])
	copy(info[IDLength:], net.ParseIP(addr.IP.String()).To4())
	copy(info[IDLength+4:], []byte{uint8(addr.Port >> 8), uint8(addr.Port & 0xff)})
	return info
}
