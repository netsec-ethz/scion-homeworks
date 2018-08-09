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
)

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

		totalRecvd = 0
		amountDelayed = 0

		filename string
	)

	/* Fetch arguments from command line */
	flag.StringVar(&serverAddress, "s", "", "Server SCION Address")
	flag.StringVar(&filename, "f", "sig_info.txt", "CryptoFileName")
	flag.Parse()

	/* Get Crypto Info */
	readSigInfo(filename)

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
		timestamp, _ := binary.Varint(receivePacketBuffer)
		if timestamp == 0 {
			break
		}
		if verifySig(receivePacketBuffer[16:n]) {
			diff_seconds := (time.Now().UnixNano() - timestamp) / 1e9
			totalRecvd += 1
			if diff_seconds > 10 {
				amountDelayed += 1
				fmt.Println("delayed")
			}
		}
	}

	fmt.Printf("Total received: %d\t Amount delayed: %d\n", totalRecvd, amountDelayed)
}

