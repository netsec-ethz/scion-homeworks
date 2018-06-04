// Dedicated client for estimating bottleneck bandwidth

package main

import (
	"flag"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/scionproto/scion/go/lib/snet"
)

const (
	PACKET_SIZE int = 4000
	PACKET_NUM int = 10
)

type checkpoint struct {
	sent, recvd int64
}

var (
	// unique id: (Time sent, time received)
	recvMap map[uint64]checkpoint
	recvLock sync.Mutex
	udpConnection *snet.Conn
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

/* Uses the checkpoints in recvMap to calculate bottleneck BW
 * Returns bandwidth sent and received in Mbps. */
func getAverageBottleneckBW() (float64, float64) {

	// Make list of tuples sorted by sent times
	var sorted []checkpoint
	for _, v := range recvMap {
		if v.recvd != 0 {
			sorted = append(sorted, v)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].sent < sorted[j].sent })

	var sent_int, recvd_int int64
	size := len(sorted)
	// Take average of intervals between consecutive send and receive.
	for i := 1; i < size; i+=1 {
		sent_int += (sorted[i].sent - sorted[i-1].sent)
		recvd_int += (sorted[i].recvd - sorted[i-1].recvd)
	}

	// Calculate BW = (#Bytes*8 / #nanoseconds) / 1e6
	bw_sent := float64(PACKET_SIZE*8*1e3) / (float64(sent_int) / float64(size))
	bw_recvd := float64(PACKET_SIZE*8*1e3) / (float64(recvd_int) / float64(size))

	return bw_sent, bw_recvd
}

// Receives replies from packets and puts them in receivemap
func recvPackets(done chan bool) {
	receivePacketBuffer := make([]byte, PACKET_SIZE + 1)
	var err error

	num := 0
	for num < PACKET_NUM {
		_, _, err = udpConnection.ReadFrom(receivePacketBuffer)
		check(err)

		ret_id, n := binary.Uvarint(receivePacketBuffer)
		recvLock.Lock()
		if val, ok := recvMap[ret_id]; ok {
			time_recvd, _ := binary.Varint(receivePacketBuffer[n:])
			val.recvd = time_recvd
			num += 1
		}
		recvLock.Unlock()
	}

	done <- true
	return
}

func main() {
	var (
		sourceAddress string
		destinationAddress string

		err    error
		local  *snet.Addr
		remote *snet.Addr

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

	udpConnection, err = snet.DialSCION("udp4", local, remote)
	check(err)

	sendPacketBuffer := make([]byte, PACKET_SIZE + 1)
	for i := 0; i < PACKET_SIZE; i+=1 {
		sendPacketBuffer[i] = 'a'
	}
	sendPacketBuffer[PACKET_SIZE] = 0

	seed := rand.NewSource(time.Now().UnixNano())

	// Create Communication Channel to Receiver
	done := make(chan bool)
	go recvPackets(done)

	// Start Send Loop
	iters := 0
	sendloop:
	for iters < 10 {
		select {
		case _ = <-done:
			break sendloop
		default:
			break;
		}
		iters += 1

		id := rand.New(seed).Uint64()
		_ = binary.PutUvarint(sendPacketBuffer, id)

		recvLock.Lock()
		recvMap[id] = checkpoint{time.Now().UnixNano(), 0}
		_, err = udpConnection.Write(sendPacketBuffer)
		recvLock.Unlock()
		check(err)
		// time.Sleep(time.Millisecond)
	}

	// Get and Display Results
	bw_sent, bw_recvd := getAverageBottleneckBW()

	fmt.Printf("\nSource: %s\nDestination: %s\n", sourceAddress, destinationAddress);
	fmt.Println("Rate sent:")
	fmt.Printf("\tBW - %.3fMbps\n", bw_sent)
	fmt.Println("Bottleneck Bandwidth estimate:")
	fmt.Printf("\tBW - %.3fMbps\n", bw_recvd)
}
