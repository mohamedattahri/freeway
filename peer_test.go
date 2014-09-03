package freeway

import (
	"net"
	"strconv"
	"testing"
)

const (
	ip   string = "127.0.0.1"
	port int    = 1666
)

var (
	id     = NewRandomID()
	dht, _ = NewDHT(id, port)
)

func TestPeerEquals(t *testing.T) {
	peer, err := NewPeerFromIP(dht, ip, port, id)
	if err != nil {
		t.Fatal(err)
	}

	other, err := NewPeerFromIP(dht, ip, port, NewRandomID())

	if peer.Equals(peer) == false {
		t.Fatal("Peer is not equal to itself")
	}

	if peer.Equals(other) == true {
		t.Fatal("Peer equality returned an unexpected result")
	}
}

func TestCompactNodeInfo(t *testing.T) {
	peer, err := NewPeerFromIP(dht, ip, port, id)
	if err != nil {
		t.Fatal(err)
	}

	infoHash := peer.CompactNodeInfo()
	recreated, err := NewPeerFromCompactNodeInfo(dht, infoHash)
	if err != nil {
		t.Fatal(err, len(infoHash))
	}

	if recreated.Equals(peer) == false {
		t.Fatal("Node recreated from compact node info was misformed", recreated, "VS", peer)
	}
}

func TestNewPeer(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}

	peer := NewPeer(dht, addr, id)

	if peer.Addr().String() != addr.String() {
		t.Fatal("Address mismatch", peer.Addr(), "VS", addr)
	}
}

func TestNewPeerFromIP(t *testing.T) {
	peer, err := NewPeerFromIP(dht, ip, port, id)
	if err != nil {
		t.Fatal(err)
	}

	if peer.Addr().IP.String() != ip {
		t.Fatal("IP address mismatch", peer.Addr().IP, "VS", ip)
	}

	if peer.Addr().Port != port {
		t.Fatal("Port number mismatch", peer.Addr().Port, "VS", port)
	}
}

func TestNewPeerFromCompactNodeInfo(t *testing.T) {
	peer, err := NewPeerFromIP(dht, ip, port, id)
	if err != nil {
		t.Fatal(err)
	}

	recreated, err := NewPeerFromCompactNodeInfo(dht, peer.CompactNodeInfo())
	if err != nil {
		t.Fatal(err)
	}
	if recreated.Equals(peer) == false {
		t.Fatal("Unable to recreate a node from its CompactNodeInfo data", recreated, "VS", peer)
	}
}
