package main

import (
	"bufio"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"flag"
	"log"
	"math/big"
	"os"
	"strconv"
)

var (
	Hash []byte
	PubKey rsa.PublicKey
	RealSignature []byte
	FakeSignature []byte
)


func main() {

	var filename string
	flag.StringVar(&filename, "f", "sig_info.txt", "FileToPutSigInfo")
	flag.Parse()

	rng := rand.Reader
	bits := 2048
	privKey, err := rsa.GenerateKey(rng, bits)
	msg := []byte("message to be signed")
	hashed := sha256.Sum256(msg)

	signature, err := rsa.SignPKCS1v15(rng, privKey, crypto.SHA256, hashed[:])
	if err != nil {
		fmt.Printf("Error from signing: %s\n", err)
		return
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	/* Hash */
	_,_ = file.WriteString(fmt.Sprintf("%x\n", hashed[:]))
	/* Correct Signature */
	_,_ = file.WriteString(fmt.Sprintf("%x\n", signature))
	/* Public Key N */
	_,_ = file.WriteString(fmt.Sprintln(privKey.PublicKey.N))
	/* Public Key E */
	_,_ = file.WriteString(fmt.Sprintln(privKey.PublicKey.E))

	fmt.Printf("Crypto info written to: %s\n", filename)
	readSigInfo(filename)
	fmt.Println("Real sig should be true and is: ", verifySig(RealSignature))
	fmt.Println("Fake sig should be false and is: ", verifySig(FakeSignature))
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
    fmt.Println(fmt.Errorf("Cannot get hash"))
  }

  /* Get Signature. */
  scanner.Scan()
  RealSignature, err = hex.DecodeString(scanner.Text())
  if err != nil {
    fmt.Println(fmt.Errorf("Cannot get signature to use"))
  }
	FakeSignature := make([]byte, len(RealSignature))
	copy(FakeSignature, RealSignature)
	FakeSignature[0] = byte('A')
	FakeSignature[10] = byte('A')

  /* Get N for RSA and create big.Int from string. */
  var big big.Int
  scanner.Scan()
  nString := scanner.Text()
  N, success := big.SetString(nString, 0)
  if !success {
    fmt.Println(nString)
    fmt.Println(fmt.Errorf("Could not create public key"))
  }

  /* Get E for RSA. */
  scanner.Scan()
  E, err := strconv.ParseInt(scanner.Text(), 10, 32)
  if err != nil {
    fmt.Println(err)
    fmt.Println(fmt.Errorf("Could not create public key"))
  }

  PubKey = rsa.PublicKey{N: N, E: int(E)}
}

func verifySig(sig []byte) bool {
  if err := rsa.VerifyPKCS1v15(&PubKey, crypto.SHA256, Hash, sig); err != nil {
    return false
  } else {
    return true
  }
}
