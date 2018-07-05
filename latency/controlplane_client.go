// A client for measuring speed (RTT and Latency)

package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/common"
	"github.com/scionproto/scion/go/lib/hpkt"
	"github.com/scionproto/scion/go/lib/overlay"
	"github.com/scionproto/scion/go/lib/sciond"
	"github.com/scionproto/scion/go/lib/scmp"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/sock/reliable"
	"github.com/scionproto/scion/go/lib/spath"
	"github.com/scionproto/scion/go/lib/spath/spathmeta"
	"github.com/scionproto/scion/go/lib/spkt"
)

const (
	NUM_ITERS = 20
	MAX_NUM_TRIES = 40
)

var Seed rand.Source

func createScmpEchoReqPkt(local *snet.Addr, remote *snet.Addr) (uint64, *spkt.ScnPkt) {
	id := rand.New(Seed).Uint64()
	info := &scmp.InfoEcho{Id: id, Seq: 0}

	scmpMeta := scmp.Meta{InfoLen: uint8(info.Len() / common.LineLen)}
	pld := make(common.RawBytes, scmp.MetaLen+info.Len())
	scmpMeta.Write(pld)
	info.Write(pld[scmp.MetaLen:])
	scmpHdr := scmp.NewHdr(scmp.ClassType{Class: scmp.C_General, Type: scmp.T_G_EchoRequest}, len(pld))

	pkt := &spkt.ScnPkt{
		DstIA:   remote.IA,
		SrcIA:   local.IA,
		DstHost: remote.Host,
		SrcHost: local.Host,
		Path:    remote.Path,
		L4:      scmpHdr,
		Pld:     pld,
	}

	return id, pkt
}


func validatePkt(pkt *spkt.ScnPkt, id uint64) (*scmp.Hdr, *scmp.InfoEcho, error) {
	scmpHdr, ok := pkt.L4.(*scmp.Hdr)
	if !ok {
		return nil, nil,
			common.NewBasicError("Not an SCMP header", nil, "type", common.TypeOf(pkt.L4))
	}
	scmpPld, ok := pkt.Pld.(*scmp.Payload)
	if !ok {
		return nil, nil,
			common.NewBasicError("Not an SCMP payload", nil, "type", common.TypeOf(pkt.Pld))
	}
	info, ok := scmpPld.Info.(*scmp.InfoEcho)
	if !ok {
		return nil, nil,
			common.NewBasicError("Not an Info Echo", nil, "type", common.TypeOf(info))
	}
	return scmpHdr, info, nil
}

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\nrandom_speedclient -s SourceSCIONAddress -d DestinationSCIONAddress")
	fmt.Println("\tProvides speed estimates (RTT and latency) from source to desination")
	fmt.Println("\tThe SCION address is specified as ISD-AS,[IP Address]:Port")
	fmt.Println("\tIf source port unspecified, a random available one will be used.")
	fmt.Println("\tExample SCION address 1-1,[127.0.0.1]:42002\n")
}

func main() {
	var (
		sourceAddress string
		destinationAddress string

		err    error
		local  *snet.Addr
		remote *snet.Addr

		scmpConnection *reliable.Conn
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

	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(local.IA, sciond.GetDefaultSCIONDPath(nil), dispatcherAddr)

	localAppAddr := &reliable.AppAddr{Addr: local.Host, Port: local.L4Port}
	scmpConnection, _, err = reliable.Register(dispatcherAddr, local.IA, localAppAddr, nil, addr.SvcNone)
	check(err)

	// Get Path to Remote
	var pathEntry *sciond.PathReplyEntry
	var options spathmeta.AppPathSet
	options = snet.DefNetwork.PathResolver().Query(local.IA, remote.IA)
	if len(options) == 0 {
		check(fmt.Errorf("Cannot find a path from source to destination"))
	}

	for _, entry := range options {
		pathEntry = entry.Entry /* Choose the first random one. */
		break
	}

	fmt.Println("Path:", pathEntry.Path.String())
	remote.Path = spath.New(pathEntry.Path.FwdPath)
	remote.Path.InitOffsets()
	remote.NextHopHost = pathEntry.HostInfo.Host()
	remote.NextHopPort = pathEntry.HostInfo.Port
	remoteAppAddr := &reliable.AppAddr{Addr: remote.NextHopHost, Port: remote.NextHopPort}
	if remote.NextHopHost == nil {
		remoteAppAddr = &reliable.AppAddr{Addr: remote.Host, Port: overlay.EndhostPort}
	}

	Seed = rand.NewSource(time.Now().UnixNano())

	// Do 5 iterations so we can use average
	var total int64 = 0
	iters := 0
	num_tries := 0
	buff := make(common.RawBytes, pathEntry.Path.Mtu)
	for iters < NUM_ITERS && num_tries < MAX_NUM_TRIES {
		num_tries += 1

		// Construct SCMP Packet
		id, pkt := createScmpEchoReqPkt(local, remote)
		pktLen, err := hpkt.WriteScnPkt(pkt, buff)
		check(err)


		time_sent := time.Now()
		_, err = scmpConnection.WriteTo(buff[:pktLen], remoteAppAddr)
		check(err)

		n, err := scmpConnection.Read(buff)
		time_received := time.Now()

		recvpkt := &spkt.ScnPkt{}
		err = hpkt.ParseScnPkt(recvpkt, buff[:n])
		check(err)
		_, info, err := validatePkt(recvpkt, id)
		check(err)

		if info.Id == id {
			diff := (time_received.UnixNano() - time_sent.UnixNano())
			total += diff
			iters += 1
			// fmt.Printf("%d: %.3fms %.3fms\n", iters, float64(diff)/1e6, float64(diff)/2e6)
		}
	}

	if iters != NUM_ITERS {
		check(fmt.Errorf("Error, exceeded maximum number of attempts"))
	}

	var difference float64 = float64(total) / float64(iters)

	fmt.Printf("\nSource: %s\nDestination: %s\n", sourceAddress, destinationAddress);
	fmt.Println("Time estimates:")
	// Print in ms, so divide by 1e6 from nano
	fmt.Printf("\tRTT - %.3fms\n", difference/1e6)
	fmt.Printf("\tLatency - %.3fms\n", difference/2e6)
}

