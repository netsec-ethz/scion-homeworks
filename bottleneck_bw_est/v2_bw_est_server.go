// Dedicated server for esitmating bottleneck bandwidth

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/sciond"
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

	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(server.IA, sciond.GetDefaultSCIONDPath(nil), dispatcherAddr)

	udpConn, err = snet.ListenSCION("udp4", server)
	check(err)

	receiveBuff := make([]byte, RECEIVE_SIZE + 1)
	var n,m int
	var num, count int64
	var zero time.Time

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
		udpConn.SetReadDeadline(time.Now().Add(5*time.Second))

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
			start := time.Now()
			_, client, err := udpConn.ReadFromSCION(receiveBuff)
			time_received := time.Now()
			fmt.Printf("Waited %d ms\n", (time_received.UnixNano() - start.UnixNano())/1e6)
			if err != nil {
				break sendloop;
			}

			/* Check to make sure it comes from clientAddr */
			if client.EqAddr(clientAddr) {
				times[count] = time_received.UnixNano()
				count += 1
			}
		}

		n = binary.PutUvarint(receiveBuff, clientId)

		/* Calculate received interval */
		var sum int64 = 0
		for i := int64(1); i < count; i+=1 {
			sum += (times[i] - times[i-1])
		}


		if count > 1 {
			/* Wont be off by more than a few nanoseconds w/ integer division */
			m = binary.PutVarint(receiveBuff[n:], sum / (count-1))
		} else {
			m = binary.PutVarint(receiveBuff[n:], 0)
		}
		receiveBuff[n+m] = 0
		fmt.Printf("Received %d packets", count)

		/* Send [unique_id, interval(ns)] then can restart */
		_, err = udpConn.WriteToSCION(receiveBuff[:n+m], clientAddr)
		check(err)
		fmt.Println("...finished")
		udpConn.SetReadDeadline(zero)
	}

}

