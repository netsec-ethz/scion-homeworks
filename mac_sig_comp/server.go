// A simple server application
package main

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	_ "crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kormat/fmt15"
	"github.com/aead/cmac"

	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/sciond"
)

// Structure of the message between client and server
type AppMessage struct {
    NumMacCompute uint32
    MacTime time.Duration
	NumSigCompute uint32
    SigTime time.Duration
}

func printUsage() {
	fmt.Println("server -s ServerSCIONAddress")
	fmt.Println("The SCION address is specified as ISD-AS,[IP Address]:Port")
	fmt.Println("Example SCION address 17-ffaa:0:1102,[192.33.93.173]:42002")
}

func Check(e error) {
	if e != nil {
		LogFatal("Fatal error. Exiting.", "err", e)
	}
}

func LogFatal(msg string, a ...interface{}) {
	log.Crit(msg, a...)
	os.Exit(1)
}

/*
 * Computes the message authentication code (MAC) based on
 * the AES-CMAC algorithm
 *
 *	Input:
 *		- key: symmetric key used to compute the MAC
 *		- message: input message that is provided by the client
 *
 *	Output:
 *		- tag: resulting MAC
 *		- err: error message
 */
func computeMAC(key []byte, message []byte) (tag string, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}

	out, err := cmac.Sum (message, block, block.BlockSize())
	if err != nil {
		return
	}

	tag = string(out)

	return
}

/*
 * Computes the signature using the RSA-based asymmetric cryptography
 *
 *	Input:
 *		- privKey: private key used to compute the signature
 *		- message: input message that is provided by the client
 *
 *	Output:
 *		- sig: resulting signature
 *		- err: error message
 */
func computeSignature(privKey *rsa.PrivateKey, message []byte) (sig string, err error) {
	var opts rsa.PSSOptions
	opts.SaltLength = rsa.PSSSaltLengthAuto // for simple example
	newhash := crypto.SHA256
	pssh := newhash.New()
	pssh.Write(message)
	hashed := pssh.Sum(nil)
	out, err := rsa.SignPSS(rand.Reader, privKey, newhash, hashed, &opts)
	if err != nil {
		fmt.Println(err)
	}
	sig = string(out)

	return
}

var (
	serverCCAddrStr string
	serverCCAddr    *snet.Addr
	err             error
	CCConn          *snet.Conn
	sciondPath      *string
	sciondFromIA    *bool
	dispatcherPath  *string
)

func main() {

	flag.StringVar(&serverCCAddrStr, "s", "", "Server SCION Address")
	id := flag.String("id", "server", "Element ID")
	logDir := flag.String("log_dir", "./logs", "Log directory")
	sciondPath = flag.String("sciond", "", "Path to sciond socket")
	sciondFromIA = flag.Bool("sciondFromIA", false, "SCIOND socket path from IA address:ISD-AS")
	dispatcherPath = flag.String("dispatcher", "/run/shm/dispatcher/default.sock",
		"Path to dispatcher socket")
	flag.Parse()

	// Setup logging
	if _, err := os.Stat(*logDir); os.IsNotExist(err) {
		os.Mkdir(*logDir, 0744)
	}
	log.Root().SetHandler(log.MultiHandler(
		log.LvlFilterHandler(log.LvlError,
			log.StreamHandler(os.Stderr, fmt15.Fmt15Format(fmt15.ColorMap))),
		log.LvlFilterHandler(log.LvlDebug,
			log.Must.FileHandler(fmt.Sprintf("%s/%s.log", *logDir, *id),
				fmt15.Fmt15Format(nil)))))
	log.Debug("Setup info:", "id", *id)

	/*
	 * Generate a 16-byte symmetric key for the AES-CMAC algorithm.
	 * 
	 * This key is from the first test vector in RFC 4493
	 *
	 * 	\url{https://tools.ietf.org/html/rfc4493#page-11
	 */
	cmacKey := []byte{0x2b, 0x7e, 0x15, 0x16,
					  0x28, 0xae, 0xd2, 0xa6,
					  0xab, 0xf7, 0x15, 0x88,
					  0x09, 0xcf, 0x4f, 0x3c}

	/*
     * Task 1: Generate a 2048-bit random RSA key pair 
     */
	privKey, err := <To be completed>
	if err != nil {
		fmt.Println (err.Error)
		os.Exit (1)
	}

	if len(serverCCAddrStr) > 0 {
		runServer(serverCCAddrStr, cmacKey, privKey)
		if err != nil {
			printUsage()
			LogFatal("Unable to start server", "err", err)
		}
	} else {
		printUsage()
		LogFatal("Error, server address needs to be specified with -s")
	}
}

const numMacCompute = 500000
const numSigCompute = 5000

/*
 * The main server routine.
 *	1. Waits and then reads the input message from the client.
 *	2. Computes MACs using the client's message as the input.
 *	   Repeats the computation for 500K times and computes
 *     the average performance.
 *	3. Computes signatures using the client's message as the input.
 *     Repeats the computation for 5K time and computes
 *	   the average performance.
 *	4. Returns the performance result back to the client.
 *	   See the AppMessage struct for more detail.
 *
 *	Input:
 *	  - serverCCAddrStr: Address at which the server listens
 *	  - cmacKey: symmetric key used for computing the AES-CMACs.
 *    - privKey: key pair used for computing the RSA signatures.
 */
func runServer(serverCCAddrStr string, cmacKey []byte, privKey *rsa.PrivateKey) {
	// Create the SCION UDP socket
	serverCCAddr, err = snet.AddrFromString(serverCCAddrStr)
	if err != nil {
		printUsage()
		LogFatal("Unable to start server", "err", err)
	}

	if *sciondFromIA {
		if *sciondPath != "" {
			LogFatal("Only one of -sciond or -sciondFromIA can be specified")
		}
		*sciondPath = sciond.GetDefaultSCIONDPath(&serverCCAddr.IA)
	} else if *sciondPath == "" {
		*sciondPath = sciond.GetDefaultSCIONDPath(nil)
	}
	log.Info("Starting server")
	snet.Init(serverCCAddr.IA, *sciondPath, *dispatcherPath)

	/*
     * Task 2: Listen on the specified address, i.e., serverCCAddr
	 *
	 *  HINT:
	 *	  - Use snet.ListenSCION:
     *
	 *			func ListenSCION(network string, laddr *Addr) (*Conn, error)
	 *
	 *			- Input Arguments:
	 *			   - 1st Argument: Specifies the network protocol (e.g., "udp4")
	 *			   - 2nd Argument: Listening address (e.g., serverCCAddr)
	 *
	 *			- Output Arguments:
	 *			   - 1st Argument: SCION connection handler
	 *			   - 2nd Argument: Specifies the error
	 */
	CCConn, err = <To be completed>
	Check(err)

	// Creates the receive buffer
	receivePacketBuffer := make([]byte, 2500)

	for {
		/*
		 * Task 3: Receive a packet from the client.
		 *
		 *	HINT:
		 * 	  - Use CCConn.ReadFromSCION:
		 *
		 *		  func (c *Conn) ReadFromSCION(b []byte) (int, *Addr, error)
		 *
		 *		  - Output Arguments:
		 *			 - 1st Arg: Number of received bytes
		 *	      	 - 2nd Arg: The client's SCION address. It will be used
		 *				        to send a packet back to the client.
		 *			 - 3rd Arg: Specifies any error
		 */
		n, clientCCAddr, err := <To be completed>
		if err != nil {
			// Todo: check error in detail, but for now simply continue
			continue
		}
		if n > 0 {
			clientCCAddrStr := clientCCAddr.String()
			fmt.Println("Received request from ", clientCCAddrStr)

			start := time.Now ()
			for i := 0; i < numMacCompute; i++ {
				/*
				 * Task 4: Compute the CMAC for the client's message
				 *
				 *  HINT:
				 *	 - We have already implemented the function for you;
				 *     You just need to find it.
				 */
				_, err := <To be completed>
				if err != nil {
					fmt.Println (err.Error)
				}
			}
			macTime := time.Since (start)

			start = time.Now ()
			for i := 0; i < numSigCompute; i++ {
				/*
				 * Task 5: Compute the RSA signature for the client's message
				 *
				 *  HINT:
				 *	 - We have already implemented the function for you;
				 *	   You just need to find it.
				 */
				_, err := <To be completed>
				if err != nil {
					fmt.Println (err.Error)
				}
			}
			sigTime := time.Since (start)

			/*
			 * Task 6: Create and send the performance report message to the client.
			 *
			 *	Requirement:
			 *	  - The message must be organized as specified by
			 *		the AppMessage struct.
			 *	  - Each field must be in network-byte order.
			 *
			 *  HINTS:
			 * 	  - Use CCConn.WriteToSCION to send a packet to the client:
			 *
			 *		  func (c *Conn) WriteToSCION(b []byte, raddr *Addr) (int, error)
			 *
			 *		  - Input Arguments:
			 *			 - 1st Argument: Buffer with the outgoing message
			 *			 - 2nd Argument: Client address (e.g., clientCCAddr)
			 *
			 *		  - Output Arguments:
			 *			 - 1st Argument: Number of bytes sent
			 *			 - 2nd Argument: Specifies the error, if any.
			 */

			<To be completed>
		}
	}
}
