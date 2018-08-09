package main

import (
	"bufio"
	"flag"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/sciond"
)

const (
	REAL_SIG = "This is my temporary real signature"
	FAKE_SIG = "This is my temporary fake signature"
	REAL_USER_THROUGHPUT = 5
	DEFAULT_PACKET_GROUP_SIZE = 10
)

var (
	Local  *snet.Addr
	Remote *snet.Addr
	Scale int
	PacketGroupSize int

	RealSignature []byte
	FakeSignature []byte
)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\nflood -s SourceSCIONAddress -d DestinationSCIONAddress")
	fmt.Println("\tThe SCION address is specified as ISD-AS,[IP Address]:Port")
}

func readSigInfo(filename string) {
  /* File contains Hash, correct signature, and N and E values from Public Key */
  file, err := os.OpenFile(filename, os.O_RDONLY, 0)
  if err != nil {
    log.Fatal(err)
  }
  defer file.Close()
  scanner := bufio.NewScanner(file)

  /* Don't need true Hash. */
  scanner.Scan()

  /* Don't need signature because will be verifying with crypto. */
  scanner.Scan()
  RealSignature, err = hex.DecodeString(scanner.Text())
  if err != nil {
    check(fmt.Errorf("Cannot get signature to use"))
  }

  /* Get N for RSA and create big.Int from string. */
  /* Don't need values for RSA PublicKey. */
  scanner.Scan()
  scanner.Scan()
}

func startSigStream(realUser bool, Wg *sync.WaitGroup) {
	var (
		rate = REAL_USER_THROUGHPUT
		iters = PacketGroupSize
		msg string
		err    error
		udpConnection *snet.Conn
	)

	if realUser {
		msg = REAL_SIG
	} else {
		msg = FAKE_SIG
		rate *= Scale
		iters *= Scale
	}

	udpConnection, err = snet.DialSCION("udp4", Local, Remote)
	check(err)

	num := make([]byte, 16)
	_ = binary.PutVarint(num, 1)
	sendPacketBuffer := append(num, []byte(msg)...)
	nap := time.Second / time.Duration(rate)
	i := 0
	for i < iters {
		_ = binary.PutVarint(sendPacketBuffer, time.Now().UnixNano())
		if i == 2 && realUser {
			time.Sleep(time.Second * 11)
		}
		_, err = udpConnection.Write(sendPacketBuffer)
		check(err)

		/* Wait for correct time interval.*/
		time.Sleep(nap)
		i += 1
	}

	Wg.Done()
}

func main() {
	var (
		sourceAddress string
		destinationAddress string

		err    error

		filename string
	)

	/* Fetch arguments from command line */
	flag.StringVar(&sourceAddress, "s", "", "Source SCION Address")
	flag.StringVar(&destinationAddress, "d", "", "Destination SCION Address")
	flag.IntVar(&Scale, "c", 5, "Constant Scale Of Attacker To Regular Throughput")
	flag.IntVar(&PacketGroupSize, "n", DEFAULT_PACKET_GROUP_SIZE, "Number Of Real User Packets To Send. Attacker Will Be Scaled")
	flag.StringVar(&filename, "f", "sig_info.txt", "CryptoFileName")
	flag.Parse()

	/* Get Crypto Info */
	readSigInfo(filename)

	/* Create the SCION UDP socket */
	if len(sourceAddress) > 0 {
		Local, err = snet.AddrFromString(sourceAddress)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, source address needs to be specified with -s"))
	}
	if len(destinationAddress) > 0 {
		Remote, err = snet.AddrFromString(destinationAddress)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, destination address needs to be specified with -d"))
	}

	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(Local.IA, sciond.GetDefaultSCIONDPath(nil), dispatcherAddr)

	var Wg sync.WaitGroup
	Wg.Add(2)

	/* Users have a constant stream rate of packets */
	go startSigStream(true, &Wg)

	/* Attackers have a constant scale of real users */
	go startSigStream(false, &Wg)

	Wg.Wait()
	udpConnection, err := snet.DialSCION("udp4", Local, Remote)
	/* Ending identifier. */
	end := make([]byte, 16)
	_ = binary.PutVarint(end, 0)
	_, err = udpConnection.Write(end)
	check(err)

	/* Figure out which stats to print. */
	fmt.Println("Done.")
}
