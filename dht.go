package freeway

import (
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"time"
)

var ip4Regex = regexp.MustCompile(`(?:[0-2]\d{0,2}\.){3}[0-2]\d{0,2}`)

// Obtain the external IP address of the current machine
// which might be hidden behind a NAT.
func externalIP() (ip string, err error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return
	}

	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	results := ip4Regex.FindSubmatch(content)
	if len(results) == 0 {
		err = errors.New("could not find any IP address in the response")
		return
	}

	ip = string(results[0])
	return
}

const (
	// Alpha represents the redundancy factor used in the DHT.
	Alpha int = 3
	// K represents the maximum capacity of a Bucket
	K int = 20
	// KeySize for a typical node ID (bits)
	KeySize int = 160
	// Freshness represents the minutes during which a node is considered fresh.
	Freshness time.Duration = 10 * time.Minute
	// Timeout delay for requests. 80% of peers should reply within 20%.
	Timeout time.Duration = 20 * time.Second
)

// DHT represents a Kadmelia-inspired Distributed Hash Table.
type DHT struct {
	id        ID
	buckets   *BucketSet // Bucket set where all the contacts will be stored
	krpc      *KRPC      // KRPC protocol agent
	localPeer *Peer      // This node
}

// LocalPeer representing the identity of the current peer on the network.
func (dht *DHT) LocalPeer() *Peer {
	return dht.localPeer
}

// FindNode uses the FIND_NODE-RPC lookup method
func (dht *DHT) FindNode(key ID, completed chan *Peer) {
	lookupTree := NewTreeMap(key)
	knownPeers := dht.buckets.ClosestPeersFrom(key)

	var startRound func(peers []*Peer, roundIndex int)
	startRound = func(peers []*Peer, roundIndex int) {
		closestPeersFound := make(chan *FindNodeResponse)

		go func(replies int) {
			/*
				After the responses of the queried nodes have been received,
				the nodes contained in the reply messages are inserted in a list.
				This list is sorted according to the distance between the nodes
				and the looked up ID.
			*/
			for i := 0; i < replies; i++ {
				reply := <-closestPeersFound
				lookupTree.state(reply.Source).replied = true
				lookupTree.addRange(reply.Peers)
				dht.buckets.InsertRange(reply.Peers)
			}
			log.Println("Peers in Buckets:", dht.buckets.Len())

			// Check if the node was found:
			if target := dht.buckets.Peer(key); target != nil {
				completed <- target
			}

			// Depending on whether the last round of FIND_NODE-RPCs revealed a new
			// closest node or not, different actions are taken.
			newClosestNodes := lookupTree.peersCloserThan(knownPeers[0])

			// If it has, then the node choses ALPHA nodes among the K newClosestNodes already
			// seen but not already queried and sends them FIND_NODE-RPCs.
			if len(newClosestNodes) > 0 {
				startRound(newClosestNodes, roundIndex+1)
				return
			}

			// Otherwise, it picks all among the K closest it has not already queried and
			// sends them FIND_NODE_RPCs.
			closest := lookupTree.unqueriedPeers()
			if len(closest) > 0 {
				startRound(closest, roundIndex+1)
				return
			}

			// The node lookup is terminated when the node has queried all the k closest nodes
			// it has discovered and gotten responses from.
			if completed != nil {
				completed <- nil
			}
			log.Println("FIND_NODE completed in", roundIndex+1, "rounds")
		}(len(peers))

		for _, peer := range peers {
			peer.FindNode(key, closestPeersFound)
			lookupTree.state(peer).queried = true
		}
	}

	lookupTree.addRange(knownPeers)
	startRound(knownPeers, 0)
}

// Bootstrap the node in the DHT by contacting peers.
func (dht *DHT) Bootstrap(peers []*Peer) error {
	if dht.buckets == nil {
		return errors.New("call Start() before boostraping")
	}

	log.Println("Boostraping DHT")
	dht.buckets.InsertRange(peers)
	dht.FindNode(dht.LocalPeer().ID(), nil)
	return nil
}

func (dht *DHT) handleIncomingRequest(query *IncomingQueryPacket) {
	sourceID, err := NewID([]byte(query.Args["id"].(string)))
	if err != nil {
		log.Println("Unable to parse the ID. Ignoring request.")
		return
	}

	// Identifying the source of the message and adding it to the buckets.
	src := NewPeer(dht, query.Source, sourceID)
	dht.buckets.Insert(src)

	// Queries with built-in support will be processed. The rest will be forwarded to the node.
	// TODO: Add forwarding support.
	switch query.QueryPacket.QueryMethod {
	case "ping":
		go dht.replyToPing(query, src)
	case "find_node":
		go dht.replyToFindNode(query, src)
	default:
		log.Println("Unsupported query \"", query.QueryPacket.QueryMethod, "\" from", src.String())
	}
}

func (dht *DHT) replyToPing(query *IncomingQueryPacket, source *Peer) {
	message := NewPingReply(query.QueryPacket.Transaction, dht.localPeer.ID())
	response := NewResponse(dht, source, message)
	if err := dht.krpc.Reply(response); err != nil {
		log.Println(err)
	}
}

func (dht *DHT) replyToFindNode(query *IncomingQueryPacket, source *Peer) {
	raw, valid := query.QueryPacket.Args["target"].(string)
	if !valid {
		log.Println("Misformatted request. Ignored.")
		return
	}

	infoHash, err := NewID([]byte(raw))
	if err != nil {
		log.Println("Unable to parse info_hash in the query.")
		return
	}
	peers := dht.buckets.ClosestPeersFrom(infoHash)
	message := NewFindNodeReply(query.QueryPacket.Transaction, dht.localPeer.ID(), peers)
	response := NewResponse(dht, source, message)
	if dht.krpc.Reply(response) != nil {
		log.Println(err)
	}
}

// Start boostraps the DHT and connects it to the network using UDP address:port.
// No need to call this method if you used NewDHT.
func (dht *DHT) Start() error {
	if dht.krpc.Listening() {
		return nil
	}

	//Creating a channel where all the incoming queries will be processed
	incomingQueries := make(chan *IncomingQueryPacket)
	go func() {
		for query := range incomingQueries {
			dht.handleIncomingRequest(query)
		}
	}()

	if err := dht.krpc.Start(incomingQueries); err != nil {
		return err
	}

	// Obtain the external IP address
	external, err := externalIP()
	if err != nil {
		return err
	}

	peer, err := NewPeerFromIP(dht, external, dht.krpc.Addr().Port, dht.id)
	if err != nil {
		return err
	}
	dht.localPeer = peer
	return nil
}

// NewDHT returns a DHT operating on UDP address:port
func NewDHT(id ID, port int) (instance *DHT, err error) {
	krpc, err := NewKRPC(port)
	if err != nil {
		return
	}

	instance = &DHT{
		krpc: krpc,
		id:   id,
	}
	instance.buckets = NewBucketSet(instance)
	return
}
