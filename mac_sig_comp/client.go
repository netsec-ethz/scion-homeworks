// A simple client application
package main

import (
	"bytes"
	"encoding/binary"
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kormat/fmt15"

	"github.com/scionproto/scion/go/lib/sciond"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/spath"
	"github.com/scionproto/scion/go/lib/spath/spathmeta"
)

// Structure of the message between client and server
type AppMessage struct {
    NumMacCompute uint32
	MacTime time.Duration
	NumSigCompute uint32
	SigTime time.Duration
}

func printUsage() {
	fmt.Println("bwtestclient -c ClientSCIONAddress -s ServerSCIONAddress -i")
	fmt.Println("A SCION address is specified as ISD-AS,[IP Address]:Port")
	fmt.Println("Example SCION address 1-1011,[192.33.93.166]:42002")
	fmt.Println("-i specifies if the client is used in interactive mode, " +
		"when true the user is prompted for a path choice")
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

// Wrapper API to select a path between the source and the destination
func ChoosePath(interactive bool, pathAlgo string, local snet.Addr, remote snet.Addr) *sciond.PathReplyEntry {
	pathMgr := snet.DefNetwork.PathResolver()
	pathSet := pathMgr.Query(local.IA, remote.IA)
	var appPaths []*spathmeta.AppPath
	var selectedPath *spathmeta.AppPath

	if len(pathSet) == 0 {
		return nil
	}

	fmt.Printf("Available paths to %v\n", remote.IA)
	i := 0
	for _, path := range pathSet {
		appPaths = append(appPaths, path)
		fmt.Printf("[%2d] %s\n", i, path.Entry.Path.String())
		i++
	}

	if interactive {
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Printf("Choose path: ")
			scanner.Scan()
			pathIndexStr := scanner.Text()
			pathIndex, err := strconv.Atoi(pathIndexStr)
			if err == nil && 0 <= pathIndex && pathIndex < len(appPaths) {
				selectedPath = appPaths[pathIndex]
				break
			}
			fmt.Printf("ERROR: Invalid path index %v, valid indices range: [0, %v]\n", pathIndex, len(appPaths)-1)
		}
	} else {
		// when in non-interactive mode, use path selection function to choose path
		selectedPath = pathSelection(pathSet, pathAlgo)
	}
	entry := selectedPath.Entry
	fmt.Printf("Using path:\n  %s\n", entry.Path.String())
	return entry
}

// Computes a path based on the indicated path algorithm
func pathSelection(pathSet spathmeta.AppPathSet, pathAlgo string) *spathmeta.AppPath {
	var selectedPath *spathmeta.AppPath
	var metric float64
	// A path selection algorithm consists of a simple comparision function selecting the best path according
	// to some path property and a metric function normalizing that property to a value in [0,1], where larger is better
	// Available path selection algorithms, the metric returned must be normalized between [0,1]:
	pathAlgos := map[string](func(spathmeta.AppPathSet) (*spathmeta.AppPath, float64)){
		"shortest": selectShortestPath,
		"mtu":      selectLargestMTUPath,
	}
	switch pathAlgo {
	case "shortest":
		log.Debug("Path selection algorithm", "pathAlgo", "shortest")
		selectedPath, metric = pathAlgos[pathAlgo](pathSet)
	case "mtu":
		log.Debug("Path selection algorithm", "pathAlgo", "MTU")
		selectedPath, metric = pathAlgos[pathAlgo](pathSet)
	default:
		// Default is to take result with best score
		for _, algo := range pathAlgos {
			cadidatePath, cadidateMetric := algo(pathSet)
			if cadidateMetric > metric {
				selectedPath = cadidatePath
				metric = cadidateMetric
			}
		}
	}
	log.Debug("Path selection algorithm choice", "path", selectedPath.Entry.Path.String(), "score", metric)
	return selectedPath
}

func selectShortestPath(pathSet spathmeta.AppPathSet) (selectedPath *spathmeta.AppPath, metric float64) {
	// Selects shortest path by number of hops
	for _, appPath := range pathSet {
		if selectedPath == nil || len(appPath.Entry.Path.Interfaces) < len(selectedPath.Entry.Path.Interfaces) {
			selectedPath = appPath
		}
	}
	metric_fn := func(rawMetric []sciond.PathInterface) (result float64) {
		hopCount := float64(len(rawMetric))
		midpoint := 7.0
		result = math.Exp(-(hopCount - midpoint)) / (1 + math.Exp(-(hopCount - midpoint)))
		return result
	}
	return selectedPath, metric_fn(selectedPath.Entry.Path.Interfaces)
}

func selectLargestMTUPath(pathSet spathmeta.AppPathSet) (selectedPath *spathmeta.AppPath, metric float64) {
	// Selects path with largest MTU
	for _, appPath := range pathSet {
		if selectedPath == nil || appPath.Entry.Path.Mtu > selectedPath.Entry.Path.Mtu {
			selectedPath = appPath
		}
	}
	metric_fn := func(rawMetric uint16) (result float64) {
		mtu := float64(rawMetric)
		midpoint := 1500.0
		tilt := 0.004
		result = 1 / (1 + math.Exp(-tilt*(mtu-midpoint)))
		return result
	}
	return selectedPath, metric_fn(selectedPath.Entry.Path.Mtu)
}

var (
	serverCCAddrStr string
	serverCCAddr    *snet.Addr
	clientCCAddrStr string
	clientCCAddr    *snet.Addr
	err             error
	CCConn          *snet.Conn
	sciondPath      string
	sciondFromIA    bool
	dispatcherPath  string
	interactive     bool
	pathAlgo        string
	msgLen       int
)

func main() {
	// Parsing the Flags
	flag.StringVar(&clientCCAddrStr, "c", "", "Client SCION Address")
	flag.StringVar(&serverCCAddrStr, "s", "", "Server SCION Address")
	flag.StringVar(&sciondPath, "sciond", "", "Path to sciond socket")
	flag.BoolVar(&sciondFromIA, "sciondFromIA", false, "SCIOND socket path from IA address:ISD-AS")
	flag.StringVar(&dispatcherPath, "dispatcher", "/run/shm/dispatcher/default.sock",
		"Path to dispatcher socket")
	flag.BoolVar(&interactive, "i", false, "Interactive mode")
	flag.StringVar(&pathAlgo, "pathAlgo", "", "Path selection algorithm / metric (\"shortest\", \"mtu\")")
	id := flag.String("id", "client", "Element ID")
	logDir := flag.String("log_dir", "./logs", "Log directory")
	flag.IntVar(&msgLen, "msg_len", 0, "Length of the message to be sent to the server")
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

	// Create SCION UDP socket
	if len(clientCCAddrStr) > 0 {
		clientCCAddr, err = snet.AddrFromString(clientCCAddrStr)
		Check(err)
	} else {
		printUsage()
		Check(fmt.Errorf("Error, client address needs to be specified with -c"))
	}
	if len(serverCCAddrStr) > 0 {
		serverCCAddr, err = snet.AddrFromString(serverCCAddrStr)
		Check(err)
	} else {
		printUsage()
		Check(fmt.Errorf("Error, server address needs to be specified with -s"))
	}

	if sciondFromIA {
		if sciondPath != "" {
			LogFatal("Only one of -sciond or -sciondFromIA can be specified")
		}
		sciondPath = sciond.GetDefaultSCIONDPath(&clientCCAddr.IA)
	} else if sciondPath == "" {
		sciondPath = sciond.GetDefaultSCIONDPath(nil)
	}
	err = snet.Init(clientCCAddr.IA, sciondPath, dispatcherPath)
	Check(err)

	var pathEntry *sciond.PathReplyEntry
	if !serverCCAddr.IA.Eq(clientCCAddr.IA) {
		/*
		 * Task 1: Path Selection
		 *
		 * To communicate with the server, we must compute a path to the server.
		 *
		 * Your job is to compute a path and refer the path by the pathEntry variable.
		 * (HINT: We have already implemented the function for you. You only need to
         *  find it and pass in the right arguments).
		 */
		pathEntry = <To be completed>
		if pathEntry == nil {
			LogFatal("No paths available to remote destination")
		}
		serverCCAddr.Path = spath.New(pathEntry.Path.FwdPath)
		serverCCAddr.Path.InitOffsets()
		serverCCAddr.NextHopHost = pathEntry.HostInfo.Host()
		serverCCAddr.NextHopPort = pathEntry.HostInfo.Port
	}


    /*
     * Task 2: Connect to the server which is specified by the serverCCAddr.
     *
     *  HINTS:
     *    - Use CCConn.DialSCION to establish a SCION connection to the server.
     *
     *        func (n *Network) DialSCION(network string, laddr *Addr, ...
     *                                    raddr *Addr) (*Conn, error) 
     *
     *        - Input Arguments:
     *           - 1st Argument: Specifies the network protocol (e.g., "udp4")
     *           - 2nd Argument: Local SCION address (e.g., clientCCAddr)
     *           - 3rd Argument: Remote SCION address (e.g., serverCCAddr)
     *
     *        - Output Arguments:
     *           - 1st Argument: SCION connection handler
     *           - 2nd Argument: Specifies the error, if any
     */
	CCConn, err = <To be completed>
	Check(err)

	go Send(CCConn)
	Read(CCConn)
}

func Send(CCConn *snet.Conn) {
	/*
	 * Task 3: Create a string that the server would use as the input
     *         for AES-CMAC and RSA-based signature computations.
	 *
	 * Write the following algorithm for creating the input string (i.e., msgStr)
     *   - If msgLen has not been specified (i.e., msgLen=0),
     *     then use the string "The quick brown fox jumps over the lazy dog" as the input.
	 *   - Otherwise, generate a random string of length msgLen that
     *	   contains only aphabets (both small and large letters, e.g., AzBc)
	 */
	msgStr := <To be completed>

	sendPacketBuffer:=[]byte(msgStr)

	var MaxTries int64 = 5
	var numtries int64 = 0
	for numtries < MaxTries {
        /*
         * Task 4: Send the above message to the server; the server address is
         *         specified by the serverCCAddr variable.
         *
         *  HINT: Use the func WriteToSCION
         */
		_, err := <To be completed>
		if err != nil {
			// Check(err)
			numtries++
			fmt.Println("Retrying")
			continue
		}

		if numtries == MaxTries && err != nil {
			Check(err)
		}
		break
	}
}

func Read(CCConn *snet.Conn) {
	receivePacketBuffer := make([]byte, 2500)

	for {
        /*
         * Task 5: Receive a packet from the server.
         *
         *  HINT: Use the func ReadFromSCION
         */
		n, serverCCAddr, err := <To be completed>
		if err != nil {
			// Todo: check error in detail, but for now simply continue
			continue
		}
		if n > 0 {
			serverCCAddrStr := serverCCAddr.String()
			fmt.Println("Received response:", serverCCAddrStr)

			// Parse the response from the server
			data := AppMessage{}
			err = binary.Read(bytes.NewReader(receivePacketBuffer), binary.BigEndian, &data)
			if err != nil {
				fmt.Println(err.Error)
				os.Exit (1)
			}

			// Print the result to the console.
			fmt.Printf("Tested using a %v-byte input\n", msgLen)
			fmt.Printf("MAC Computation (AES-CMAC): %.2f\tus per operation (Averaged over %v runs)\n",
						float64(data.MacTime.Nanoseconds()/1000)/float64(data.NumMacCompute),
						data.NumMacCompute)
			fmt.Printf("Sig Computation (RSA)     : %.2f\tus per operation (Averaged over %v runs)\n",
						float64(data.SigTime.Nanoseconds()/1000)/float64(data.NumSigCompute),
						data.NumSigCompute)
			break
		}
	}
}
