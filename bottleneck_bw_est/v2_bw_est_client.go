// Dedicated client for estimating bottleneck bandwidth

package main

import (
	"flag"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/snet"
)

const (
	PACKET_SIZE int = 4000
	PACKET_NUM int = 10
	NUM_TRIES int = 3
)


func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\nbw_est_client -s SourceSCIONAddress -d DestinationSCIONAddress")
	fmt.Println("\tProvides bottleneck bandwidth estimation from source to dedicated destination using simplified packet pair algorithm")
	fmt.Println("\tThe SCION address is specified as ISD-AS,[IP Address]:Port")
	fmt.Println("\tIf source port unspecified, a random available one will be used")
	fmt.Println("\tExample SCION address 1-1,[127.0.0.1]:42002\n")
}

func main() {
	var (
		sourceAddress string
		destinationAddress string

		err    error
		local  *snet.Addr
		remote *snet.Addr
		udpConn *snet.Conn

		uid uint64
		times []int64
	)

	// Fetch arguments from command line
	flag.StringVar(&sourceAddress, "s", "", "Source SCION Address")
	flag.StringVar(&destinationAddress, "d", "", "Destination SCION Address")
	flag.Parse()

	// Create the SCION UDP socket
	if len(sourceAddress) > 0 {
		local, err = snet.AddrFromString(sourceAddress)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, source address needs to be specified with -s"))
	}
	if len(destinationAddress) > 0 {
		remote, err = snet.AddrFromString(destinationAddress)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, destination address needs to be specified with -d"))
	}

	sciondAddr := fmt.Sprintf("/run/shm/sciond/sd%d-%d.sock", local.IA.I, local.IA.A)
	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(local.IA, sciondAddr, dispatcherAddr)

	udpConn, err = snet.DialSCION("udp4", local, remote)
	check(err)

	times = make([]int64, PACKET_NUM)
	sendBuff := make([]byte, PACKET_SIZE + 1)

	/* Send initialization with timeout NUM_TRIES times */
	seed := rand.NewSource(time.Now().UnixNano())
	i := 0
	for i < NUM_TRIES {
		n := binary.PutVarint(sendBuff, 1)
		uid = rand.New(seed).Uint64()
		m := binary.PutUvarint(sendBuff[n:], uid)
		k := binary.PutVarint(sendBuff[n+m:], int64(PACKET_NUM))
		sendBuff[n+m+k] = 0

		/* Send [1, unique_id, #packets] */
		_, err = udpConn.Write(sendBuff[:n+m+k])
		check(err)

		/* Read [1, same_id] */
		udpConn.SetReadDeadline(time.Now().Add(2*time.Second))
		_, err = udpConn.Read(sendBuff)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				i += 1
				continue
			} else {
				check(err)
			}
		}

		num, n := binary.Varint(sendBuff)
		id, _ := binary.Uvarint(sendBuff[n:])
		if (num == 1) && (uid == id) {
			break
		}
		i += 1
	}

	if i == NUM_TRIES {
		check(fmt.Errorf("Error, exceeded maximum number of initialization attempts"))
	}

	/* Initialize data packet */
	for i := 0; i < PACKET_SIZE; i += 1 {
		sendBuff[i] = 'a'
	}
	sendBuff[PACKET_SIZE] = 0

	i = 0
	for i < PACKET_NUM {
		times[i] = time.Now().UnixNano()
		_, err = udpConn.Write(sendBuff)
		check(err)
		i += 1
		time.Sleep(time.Microsecond)
	}

	/* Remove read deadline */
	var zero time.Time
	udpConn.SetReadDeadline(zero)

	/* Read [unique_id, interval(ns)] */
	_, err = udpConn.Read(sendBuff)
	check(err)
	id, n := binary.Uvarint(sendBuff)
	if (uid != id) {
		check(fmt.Errorf("Error, did not receive the correct id back"))
	}

	recvd_int, _ := binary.Varint(sendBuff[n:])

	/* Calculate send and received bw */
	var sum int64 = 0
	for _, i := range times {
		sum += i
	}
	sent_int := sum / int64(PACKET_NUM)

	/* Calculate BW (Mbps) = (#Bytes*8 / #nanoseconds) / 1e6 */
	bw_sent := float64(PACKET_SIZE*8*1e3) / float64(sent_int)

	bw_recvd := float64(PACKET_SIZE*8*1e3) / float64(recvd_int)

	// Display Results
	fmt.Printf("\nSource: %s\nDestination: %s\n", sourceAddress, destinationAddress);
	fmt.Println("Rate sent:")
	fmt.Printf("\tBW - %.3fMbps\n", bw_sent)
	fmt.Println("Bottleneck Bandwidth estimate:")
	fmt.Printf("\tBW - %.3fMbps\n", bw_recvd)
}
