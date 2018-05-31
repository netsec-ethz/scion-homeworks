// Dedicated server for measuring speed (RTT and Latency)

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/scionproto/scion/go/lib/snet"
)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\ndedicated_speedserver -s ServerSCIONAddress")
	fmt.Println("\tListens for incoming connections and responds back to them right away")
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

	receivePacketBuffer := make([]byte, 2500)
	for {
		n, clientAddress, err := udpConnection.ReadFrom(receivePacketBuffer)
		check(err)

		// Packet received, send back response to same client
		_, err = udpConnection.WriteTo(receivePacketBuffer[:n], clientAddress)
		check(err)
		fmt.Println("Received connection from", clientAddress)
	}
}

