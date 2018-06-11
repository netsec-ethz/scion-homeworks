// Dedicated server for esitmating bottleneck bandwidth

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
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
		err    error

		serverAddr string
		server *snet.Addr
		udpConn *snet.Conn

		times []int64
		clientAddr *snet.Addr
		clientId uint64
		num_packets int64

	)

	// Fetch arguments from command line
	flag.StringVar(&serverAddr, "s", "", "Server SCION Address")
	flag.Parse()

	// Create the SCION UDP socket
	if len(serverAddr) > 0 {
		server, err = snet.AddrFromString(serverAddr)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, server address needs to be specified with -s"))
	}

	sciondAddr := fmt.Sprintf("/run/shm/sciond/sd%d-%d.sock", server.IA.I, server.IA.A)
	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(server.IA, sciondAddr, dispatcherAddr)

	udpConn, err = snet.ListenSCION("udp4", server)
	check(err)

	receiveBuff := make([]byte, RECEIVE_SIZE + 1)
	var n,m int
	var num, count int64

	for {
		/* Receive [1, unique_id, #packets] */
		m, clientAddr, err = udpConn.ReadFromSCION(receiveBuff)
		num, n = binary.Varint(receiveBuff)

		/* Initialize connection */
		if num == 1 {
			clientId, m = binary.Uvarint(receiveBuff[n:])
			num_packets, _ = binary.Varint(receiveBuff[n+m:])
			times = make([]int64, num_packets)

			/* Send ack as [1, same_id] */
			n = binary.PutVarint(receiveBuff, 1)
			m = binary.PutUvarint(receiveBuff[n:], clientId)
			receiveBuff[n+m] = 0
			_, err = udpConn.WriteToSCION(receiveBuff[:n+m], clientAddr)
			check(err)
		} else {
			continue
		}
		fmt.Println("Beginning bandwidth test with", clientAddr, "for", num_packets, "packets.")
		timer := time.NewTimer(4 * time.Second).C
		udpConn.SetReadDeadline(time.Now().Add(3*time.Second))

		count = 0
		sendloop:
		for {
			select {
			case _ = <-timer:
				break sendloop
			default:
				if count >= num_packets {
					break sendloop
				}
			}

			/* Wait for new packet */
			_, client, err := udpConn.ReadFromSCION(receiveBuff)
			time_received := time.Now().UnixNano()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					break;
				} else {
					check(err)
				}
			}

			/* Check to make sure it comes from clientAddr */
			if client.EqAddr(clientAddr) {
				times[count] = time_received
				count += 1
			}
		}

		n = binary.PutUvarint(receiveBuff, clientId)

		/* Calculate received interval */
		var sum int64 = 0
		for i := int64(1); i < count; i+=1 {
			sum += (times[i] - times[i-1])
		}


		if count != 0 {
			/* Wont be off by more than a few nanoseconds w/ integer division */
			m = binary.PutVarint(receiveBuff[n:], sum / (count-1))
		} else {
			m = binary.PutVarint(receiveBuff[n:], 0)
		}
		receiveBuff[n+m] = 0

		/* Send [unique_id, interval(ns)] then can restart */
		_, err = udpConn.WriteToSCION(receiveBuff[:n+m], clientAddr)
		check(err)
		fmt.Println("...finished")
		var zero time.Time
		udpConn.SetReadDeadline(zero)
	}

}

