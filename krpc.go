package freeway

import (
	"bytes"
	"log"
	"net"
	"time"

	bencode "github.com/jackpal/bencode-go"
)

const (
	ipv        string = "udp4"
	bufferSize int    = 4096
)

const (
	genericErrorCode  int = iota + 201
	serverErrorCode       = iota
	protocolErrorCode     = iota
	methodUnknownCode     = iota
)

const (
	krpcQuery    = "q"
	krpcResponse = "r"
	krpcError    = "e"
)

// KRPC server listening on UDP
type KRPC struct {
	listening       bool                // Indicates whether the UPD server is in listening mode
	socket          *net.UDPConn        // Socket connection on which the node is listening
	port            int                 // Port number on which the parent node is listening
	transactions    map[string]*Request // Requests awaiting responses
	bytesWritten    uint64              // Total number of bytes written
	bytesRead       uint64              // Total number of bytes received
	packetsSent     uint64              // Total number of packets sent
	packetsReceived uint64              // Total number of packets receveid
	timeouts        uint64              // Total number of timeouts recorded
	requests        uint64              // Total number of requests sent
	responses       uint64              // Total number of responses received
	inquiries       uint64              // Total number of requests received
	replies         uint64              // Total number of responses sent
	errors          uint64              // Total number of errors received
	broadcastsSent  uint64              // Total number of broadcasts sent
}

// Listening returns a value indicating whether the server is running
func (krpc *KRPC) Listening() bool {
	return krpc.listening
}

// Address on which the server is listening
func (krpc *KRPC) Addr() *net.UDPAddr {
	if krpc.listening {
		return krpc.socket.LocalAddr().(*net.UDPAddr)
	}

	return nil
}

// Send a request after bencoding its message
func (krpc *KRPC) Send(req *Request) (err error) {
	krpc.transactions[req.TransactionID()] = req
	writer := new(bytes.Buffer)
	bencode.Marshal(writer, *req.message)
	written, err := krpc.socket.WriteToUDP(writer.Bytes(), req.dest.Addr())
	req.requestTime = time.Now()
	krpc.bytesWritten += uint64(written)
	krpc.packetsSent++
	krpc.requests++
	if err != nil {
		return
	}

	// Maintenance : removing keys for timed out requests.
	// Transaction map will not keep growing undefinitely.
	go func(request *Request) {
		time.Sleep(Timeout)
		if _, exists := krpc.transactions[request.TransactionID()]; exists {
			delete(krpc.transactions, request.TransactionID())
			krpc.timeouts++
		}
	}(req)
	return
}

// Reply sends a reponse to a query that was processed.
func (krpc *KRPC) Reply(resp *Response) (err error) {
	writer := new(bytes.Buffer)
	bencode.Marshal(writer, *resp.message)
	written, err := krpc.socket.WriteToUDP(writer.Bytes(), resp.dest.Addr())
	krpc.bytesWritten += uint64(written)
	krpc.packetsSent++
	krpc.replies++
	if err != nil {
		log.Println("Troubles sending message", err)
		return err
	}

	return
}

func (krpc *KRPC) handleRequest(incomingQueries chan *IncomingQueryPacket, data map[string]interface{}, rAddr *net.UDPAddr) {
	args, valid := data["a"].(map[string]interface{})
	if !valid {
		log.Println("Received an invalid request. Ignored")
		return
	}

	// Identifying the source of the request
	incomingRequest := &IncomingQueryPacket{
		QueryPacket: QueryPacket{
			Args:        args,
			QueryMethod: data["q"].(string),
			PacketType:  data["y"].(string),
			Transaction: data["t"].(string),
		},
		Source: rAddr,
	}
	incomingQueries <- incomingRequest
}

func (krpc *KRPC) handleResponse(data map[string]interface{}, rAddr *net.UDPAddr) {
	// Determine wether the received packet is the response to a request issued
	// from the DHT.
	response := &IncomingReplyPacket{
		ReplyPacket: ReplyPacket{
			NamedReturns: data["r"].(map[string]interface{}),
			PacketType:   data["y"].(string),
			Transaction:  data["t"].(string),
		},
		Source: rAddr,
	}

	if request, exists := krpc.transactions[response.ReplyPacket.Transaction]; exists {
		defer delete(krpc.transactions, request.TransactionID())
		request.responseTime = time.Now()
		request.responseChan <- response
	}
}

func (krpc *KRPC) handleError(buffer *bytes.Buffer, rAddr *net.UDPAddr) {
	response := &IncomingReplyPacket{}
	if err := bencode.Unmarshal(buffer, response); err != nil {
		log.Println("Could not parse received data as a ResponsePacket")
		return
	}

	// TODO: Handle each error type
	// switch response.Error[0] {
	// case genericErrorCode:
	// case serverErrorCode:
	// case protocolErrorCode:
	// case methodUnknownCode:
	// }

	// Determine wether the received packet is the response to a request issued
	// from the DHT.
	if request, exists := krpc.transactions[response.Transaction]; exists {
		delete(krpc.transactions, request.TransactionID())
	}
}

func (krpc *KRPC) handleIncoming(incomingQueries chan *IncomingQueryPacket, read int, data []byte, addr *net.UDPAddr) {
	krpc.packetsReceived++
	krpc.bytesRead += uint64(read)

	// The goal here is to cast the received data as a map, and determine
	// whether it's a query, a response or an error by looking at the value of
	// "y" as defined by the KRPC protocol.

	buffer := bytes.NewBuffer(data[:read])
	raw, err := bencode.Decode(buffer)
	if err != nil {
		log.Println("Unable to decode bencoded string. Packet will be ignored", buffer, read)
		return
	}

	typed, valid := raw.(map[string]interface{})
	if !valid {
		// TODO: Blacklisting ?
		log.Println("Invalid packet received")
		return
	}

	switch typed["y"].(string) {
	case krpcQuery:
		krpc.inquiries++
		krpc.handleRequest(incomingQueries, typed, addr)
	case krpcResponse:
		krpc.responses++
		krpc.handleResponse(typed, addr)
	case krpcError:
		krpc.errors++
		krpc.handleError(buffer, addr)
	default:
		log.Println("Unable to process received message", buffer)
	}
}

// Start listening on the network.
func (krpc *KRPC) Start(incomingQueries chan *IncomingQueryPacket) (err error) {
	krpc.socket, err = net.ListenUDP(ipv, &net.UDPAddr{Port: krpc.port})
	if err != nil {
		return err
	}
	krpc.socket.SetReadBuffer(bufferSize)

	go func() {
		for {
			buffer := make([]byte, bufferSize)
			read, addr, err := krpc.socket.ReadFromUDP(buffer[0:])
			if err != nil {
				log.Print(err)
				continue
			}

			// Processing of messages is handed over to a go-routine.
			go krpc.handleIncoming(incomingQueries, read, buffer, addr)
		}
	}()
	krpc.listening = true

	log.Println("Listening on UDP", krpc.socket.LocalAddr())
	return
}

// Stop listening
func (krpc *KRPC) Stop() {
	if krpc.socket != nil && krpc.listening {
		krpc.listening = false
		krpc.socket.Close()
	}
}

// NewKRPC returns a new KRPC server
func NewKRPC(port int) (*KRPC, error) {
	instance := &KRPC{
		port:         port,
		transactions: make(map[string]*Request),
	}
	return instance, nil
}
