// Dedicated client for estimating bottleneck bandwidth

package main

import (
	"flag"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/scionproto/scion/go/lib/sciond"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/spath"
	"github.com/scionproto/scion/go/lib/spath/spathmeta"
)

const (
	DEFAULT_PACKET_SIZE int = 8000
	DEFAULT_PACKET_NUM int = 10
	NUM_TRIES int = 3
)

var (
	PACKET_SIZE int
	PACKET_NUM int
)


func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\nbw_est_client -s SourceSCIONAddress -d DestinationSCIONAddress [-p PacketSize] [-n PacketNum]")
	fmt.Println("\tProvides bottleneck bandwidth estimation from source to dedicated destination using simplified packet pair algorithm")
	fmt.Println("\tThe SCION address is specified as ISD-AS,[IP Address]:Port")
	fmt.Println("\tIf source port unspecified, a random available one will be used")
	fmt.Println("\tExample SCION address 1-1,[127.0.0.1]:42002")
	fmt.Println("\tIf packet size (in bytes) and packet num are unspecified, defaults are used.\n")
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

	/* Fetch arguments from command line */
	flag.StringVar(&sourceAddress, "s", "", "Source SCION Address")
	flag.StringVar(&destinationAddress, "d", "", "Destination SCION Address")
	flag.IntVar(&PACKET_SIZE, "p", DEFAULT_PACKET_SIZE, "Packet Size")
	flag.IntVar(&PACKET_NUM, "n", DEFAULT_PACKET_NUM, "Packet Num")
	flag.Parse()

	/* Create the SCION UDP socket */
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

	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(local.IA, sciond.GetDefaultSCIONDPath(nil), dispatcherAddr)

	/* Register local application */
	udpConn, err = snet.ListenSCION("udp4", local)
	check(err)

	/* Get Path to Remote */
	var pathEntry *sciond.PathReplyEntry
	var options spathmeta.AppPathSet
	options = snet.DefNetwork.PathResolver().Query(local.IA, remote.IA)
	if len(options) == 0 {
		check(fmt.Errorf("Cannot find a path from source to destination"))
	}

	var biggest string
	for k, entry := range options {
		if k.String() > biggest {
			pathEntry = entry.Entry /* Choose the first random one. */
		}
	}

	fmt.Println("\nPath:", pathEntry.Path.String())
	remote.Path = spath.New(pathEntry.Path.FwdPath)
	remote.Path.InitOffsets()
	remote.NextHopHost = pathEntry.HostInfo.Host()
	remote.NextHopPort = pathEntry.HostInfo.Port

	times = make([]int64, PACKET_NUM)
	sendBuff := make([]byte, PACKET_SIZE + 1)

	/* Send initialization with timeout NUM_TRIES times */
	seed := rand.NewSource(time.Now().UnixNano())
	/* No read deadline */
	var zero time.Time
	i := 0
	for i < NUM_TRIES {
		n := binary.PutVarint(sendBuff, 1)
		uid = rand.New(seed).Uint64()
		m := binary.PutUvarint(sendBuff[n:], uid)
		k := binary.PutVarint(sendBuff[n+m:], int64(PACKET_NUM))
		sendBuff[n+m+k] = 0

		/* Send [1, unique_id, #packets] */
		_, err = udpConn.WriteToSCION(sendBuff[:n+m+k], remote)
		check(err)

		/* Read [1, same_id] */
		udpConn.SetReadDeadline(time.Now().Add(2*time.Second))
		_, err = udpConn.Read(sendBuff)
		if err != nil {
			i += 1
			continue
		}

		udpConn.SetReadDeadline(zero)

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
		_, err = udpConn.WriteToSCION(sendBuff, remote)
		check(err)
		i += 1
		time.Sleep(time.Millisecond)
	}


	/* Read [unique_id, interval(ns)] */
	_, err = udpConn.Read(sendBuff)
	check(err)
	id, n := binary.Uvarint(sendBuff)
	if (uid != id) {
		check(fmt.Errorf("Error, did not receive the correct id back.\nSent: %d\nReceived: %d\n", uid, id))
	}

	recvd_int, _ := binary.Varint(sendBuff[n:])

	/* Calculate send and received bw */
	var sum int64 = 0
	for i := 1; i < PACKET_NUM; i+=1 {
		sum += (times[i] - times[i-1])
	}
	sent_int := sum / int64(PACKET_NUM - 1)

	/* Calculate BW (Mbps) = (#Bytes*8 / #nanoseconds) / 1e6 */
	bw_sent := float64(PACKET_SIZE*8*1e3) / float64(sent_int)
	var bw_recvd float64
	if recvd_int != 0 {
		bw_recvd = float64(PACKET_SIZE*8*1e3) / float64(recvd_int)
	} else {
		fmt.Println("\nNot enough packets successfully received.")
		bw_recvd = 0
	}

	/* Display Results */
	fmt.Printf("\nSource: %s\nDestination: %s\n", sourceAddress, destinationAddress);
	fmt.Println("Rate sent:")
	fmt.Printf("\tBW - %.3fMbps\n", bw_sent)
	fmt.Println("Bottleneck Bandwidth estimate:")
	fmt.Printf("\tBW - %.3fMbps\n", bw_recvd)
}
