package freeway

import (
	"net"
)

// The content of an RPC reponse UDP packet
type ReplyPacket struct {
	NamedReturns map[string]interface{} `bencode:"r"`
	PacketType   string                 `bencode:"y"`
	Transaction  string                 `bencode:"t"`
}

type IncomingReplyPacket struct {
	ReplyPacket
	Source *net.UDPAddr
}

func NewReplyPacket(transaction string, args map[string]interface{}) *ReplyPacket {
	return &ReplyPacket{
		NamedReturns: args,
		PacketType:   krpcReply,
		Transaction:  transaction,
	}
}

func NewPingReply(transaction string, localPeerID ID) *ReplyPacket {
	args := make(map[string]interface{})
	args["id"] = localPeerID.String()
	return NewReplyPacket(transaction, args)
}

func NewFindNodeReply(transaction string, localPeerID ID, peers []*Peer) *ReplyPacket {
	args := make(map[string]interface{})
	args["id"] = localPeerID.String()
	args["nodes"] = string(PeerArray(peers).CompactInfo())
	return NewReplyPacket(transaction, args)
}

type Response struct {
	dht     *DHT
	dest    *Peer
	message *ReplyPacket
}

func NewResponse(dht *DHT, dest *Peer, message *ReplyPacket) *Response {
	return &Response{
		dht:     dht,
		dest:    dest,
		message: message,
	}
}

// Structure of a FindNode RPC-reply.
type FindNodeResponse struct {
	Source *Peer
	Peers  []*Peer
}
