package freeway

import (
	"net"
	"time"
)

// The content of a KRPC request UDP packet.
type QueryPacket struct {
	Args        map[string]interface{} `bencode:"a"`
	QueryMethod string                 `bencode:"q"`
	PacketType  string                 `bencode:"y"`
	Transaction string                 `bencode:"t"`
}

type IncomingQueryPacket struct {
	QueryPacket // Anonymous field
	Source      *net.UDPAddr
}

// Returns a message instance fit for a KRPC query.
func NewQuery(method string, args map[string]interface{}) (query *QueryPacket) {
	return &QueryPacket{
		Args:        args,
		QueryMethod: method,
		PacketType:  krpcQuery,
		Transaction: NewRandomID().String(),
	}
}

// Most basic KRPC query.
func NewPingQuery(queryingNodeID ID) *QueryPacket {
	args := make(map[string]interface{})
	args["id"] = queryingNodeID.String()
	return NewQuery("ping", args)
}

// Find node is used to find the contact information for the node with targetNodeID
func NewFindNodeQuery(queryingNodeID, targetNodeID ID) *QueryPacket {
	args := make(map[string]interface{})
	args["id"] = queryingNodeID.String()
	args["target"] = targetNodeID.String()
	return NewQuery("find_node", args)
}

// Get peers associated with a torrent infohash
func NewGetPeersQuery(queryingNodeID, infoHash ID) *QueryPacket {
	args := make(map[string]interface{})
	args["id"] = queryingNodeID.String()
	args["info_hash"] = infoHash.String()
	return NewQuery("get_peers", args)
}

type Request struct {
	dht          *DHT
	dest         *Peer
	message      *QueryPacket
	responseChan chan *IncomingReplyPacket
	requestTime  time.Time
	responseTime time.Time
}

func (req *Request) TransactionID() string {
	return req.message.Transaction
}

func NewRequest(dht *DHT, dest *Peer, message *QueryPacket, responseChan chan *IncomingReplyPacket) *Request {
	return &Request{
		dht:          dht,
		dest:         dest,
		message:      message,
		responseChan: responseChan,
	}
}
