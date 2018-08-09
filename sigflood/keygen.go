package main

import (
	"bufio"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"flag"
	"log"
	"os"
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
	readSigInfo(filename)
}

func readSigInfo(filename string) {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
}
