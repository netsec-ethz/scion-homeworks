// Dedicated server for esitmating bottleneck bandwidth

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/scionproto/scion/go/lib/snet"
)

const (
	RECEIVE_SIZE int = 50000
)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\nbw_est_server -s ServerSCIONAddress")
	fmt.Println("\tListens for incoming connections and responds back to them right away with the time received")
	fmt.Println("\tThe SCION address is specified as ISD-AS,[IP Address]:Port")
	fmt.Println("\tIf server listening port unspecified, a random available one will be used")
	fmt.Println("\tExample SCION address 1-1,[127.0.0.1]:42002\n")
}

func main() {
	var (
		serverAddress string

		err    error
		server *snet.Addr

		udpConnection *snet.Conn
	)

	// Fetch arguments from command line
	flag.StringVar(&serverAddress, "s", "", "Server SCION Address")
	flag.Parse()

	// Create the SCION UDP socket
	if len(serverAddress) > 0 {
		server, err = snet.AddrFromString(serverAddress)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, server address needs to be specified with -s"))
	}

	sciondAddr := fmt.Sprintf("/run/shm/sciond/sd%d-%d.sock", server.IA.I, server.IA.A)
	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(server.IA, sciondAddr, dispatcherAddr)

	udpConnection, err = snet.ListenSCION("udp4", server)
	check(err)

	receivePacketBuffer := make([]byte, RECEIVE_SIZE + 1)
	for {
		n, clientAddress, err := udpConnection.ReadFrom(receivePacketBuffer)
		time_recvd := time.Now().UnixNano()
		check(err)

		_, size := binary.Uvarint(receivePacketBuffer)
		n = binary.PutVarint(receivePacketBuffer[size:], time_recvd)
		// Packet received, send back response to same client with time
		_, err = udpConnection.WriteTo(receivePacketBuffer[:n+size], clientAddress)
		check(err)
	}
}

