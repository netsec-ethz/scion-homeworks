package main

import (
	"bufio"
	"crypto"
	"crypto/rsa"
	"flag"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/sciond"
)

var (
	Hash []byte
	PubKey rsa.PublicKey

	Method int
	RequestHandler defense


	TotalRecvd = 0
	AmountDelayed = 0

	/* Binning Vars */
	ReceivedThreshold = 100
	PercentageThreshold = 0.0
	MaxPaths = 50
	PathPos = 0
	SavedPaths = make([]string, MaxPaths)
)

type defense func([]byte, int) bool

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func printUsage() {
	fmt.Println("\nserver -s ServerSCIONAddress")
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

	/* Get Hash. */
	scanner.Scan()
	Hash, err = hex.DecodeString(scanner.Text())
	if err != nil {
		check(fmt.Errorf("Cannot get hash"))
	}

	/* Don't need signature because will be verifying with crypto. */
	scanner.Scan()

	/* Get N for RSA and create big.Int from string. */
	var big big.Int
	scanner.Scan()
	nString := scanner.Text()
	N, success := big.SetString(nString, 0)
	if !success {
		fmt.Println(nString)
		check(fmt.Errorf("Could not create public key"))
	}

	/* Get E for RSA. */
	scanner.Scan()
	E, err := strconv.ParseInt(scanner.Text(), 10, 32)
	if err != nil {
		fmt.Println(err)
		check(fmt.Errorf("Could not create public key"))
	}

	PubKey = rsa.PublicKey{N: N, E: int(E)}
}

func defaultRequestHandler(req []byte, n int) bool {
	timestamp, _ := binary.Varint(req)
	if timestamp == 0 {
		return true;
	}
	if verifySig(req[TIMESTAMP_SIZE+PAYLOAD_SIZE:n]) {
		diff_seconds := (time.Now().UnixNano() - timestamp) / 1e9
		TotalRecvd += 1
		if diff_seconds > 10 {
			AmountDelayed += 1
			fmt.Println("delayed")
		}
	}
	return false
}

func BinningRequestHandler(req []byte, n int) bool {
	timestamp, _ := binary.Varint(req)
	if timestamp == 0 {
		return true;
	}

	path := string(req[TIMESTAMP_SIZE:TIMESTAMP_SIZE+PAYLOAD_SIZE])
	/* Can optionally choose to serve based on bin, if desired. */
	SavedPaths[PathPos] = path
	PathPos += 1
	if PathPos >= MaxPaths {
		PathPos = 0
	}

	correct := verifySig(req[TIMESTAMP_SIZE+PAYLOAD_SIZE:n])
	if correct {
		diff_seconds := (time.Now().UnixNano() - timestamp) / 1e9
		TotalRecvd += 1
		if diff_seconds > 10 {
			AmountDelayed += 1
			fmt.Println("delayed")
		}
	}

	/* Use running average to determine if we are in DOS and want to find attacker. */
	var i = -1
	if !correct {
		i = 1
	}

	PercentageThreshold = PercentageThreshold*0.8 + float64(i)*0.2

	if PercentageThreshold > 0 && TotalRecvd > ReceivedThreshold {
		/* */
		attacker := FindAttacker(SavedPaths, 1)[0].K
		/* Can choose to adjust binning measures to limit the attacker. */
		fmt.Println("The attacker is from AS:", attacker)
	}

	return false
}

func setupMethod(method string) {
	Method, in_list := METHODS[method]
	if !in_list {
		Method = 0
		method = "normal"
	}

	RequestHandler = defaultRequestHandler
	switch Method {
		/* Normal. */
		case 0:
			break
		/* Binning. */
		case 1:
			break
		case 2:
			break
		default:
			break
	}

	fmt.Printf("\nRunning the %s DOS server method.\n\n", method)
}

func verifySig(sig []byte) bool {
	if err := rsa.VerifyPKCS1v15(&PubKey, crypto.SHA256, Hash, sig); err != nil {
		// fmt.Println("Fake")
		return false
	} else {
		// fmt.Println("Real")
		return true
	}
}

func main() {
	var (
		serverAddress string
		err    error
		server *snet.Addr
		udpConnection *snet.Conn

		filename string
	)

	/* Fetch arguments from command line */
	flag.StringVar(&serverAddress, "s", "", "Server SCION Address")
	flag.StringVar(&filename, "f", "sig_info.txt", "CryptoFileName")
	m := flag.String("m", "normal", "SigFloodMethod")
	flag.Parse()

	/* Get Crypto Info */
	readSigInfo(filename)
	setupMethod(*m)

	/* Create the SCION UDP socket */
	if len(serverAddress) > 0 {
		server, err = snet.AddrFromString(serverAddress)
		check(err)
	} else {
		printUsage()
		check(fmt.Errorf("Error, server address needs to be specified with -s"))
	}

	dispatcherAddr := "/run/shm/dispatcher/default.sock"
	snet.Init(server.IA, sciond.GetDefaultSCIONDPath(nil), dispatcherAddr)

	udpConnection, err = snet.ListenSCION("udp4", server)
	check(err)


	receivePacketBuffer := make([]byte, 2060)
	for {
		n, _, err := udpConnection.ReadFrom(receivePacketBuffer)
		check(err)
		done := RequestHandler(receivePacketBuffer, n)
		if done {
			break
		}
	}

	fmt.Printf("Total received: %d\t Amount delayed: %d\n", TotalRecvd, AmountDelayed)
}

