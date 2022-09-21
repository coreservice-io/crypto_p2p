package addrmgr

import (
	"container/list"
	crand "crypto/rand"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/crypto_p2p/protocol"
	"github.com/coreservice-io/log"
)

const (
	newBucketCount      = 1024
	triedBucketCount    = 8
	dumpAddressInterval = time.Minute * 10
)

type serializedAddrManager struct {
	Version      int
	Key          [32]byte
	Addresses    []*serializedKnownAddress
	NewBuckets   [newBucketCount][]string // string is NetAddressKey
	TriedBuckets [triedBucketCount][]string
}

type serializedKnownAddress struct {
	Addr        string // network address from addr msg.
	Src         string // current peer.
	Attempts    int    // times before connected success.
	TimeStamp   int64  // time recently try connecting.
	LastAttempt int64
	LastSuccess int64
	// no refcount or tried, that is available from context.
}

type KnownAddress struct {
	na          string
	srcAddr     string
	attempts    int
	lastattempt time.Time
	lastsuccess time.Time
	tried       bool // mark try connection or false. move tried addr to TriedBuckets.
	refs        int  // reference count of new buckets
}

type AddrManager struct {
	mtx        sync.RWMutex                             // am object lock, ensure safe
	peersFile  string                                   // store address filename
	lookupFunc func(string) ([]net.IP, error)           // for dns lookup.
	rand       *rand.Rand                               // real rand generation.
	key        [32]byte                                 // 32bytes rand number seq for two buckets index.
	addrIndex  map[string]*KnownAddress                 // address key for all addrs.
	addrNew    [newBucketCount]map[string]*KnownAddress // new addrs.
	addrTried  [triedBucketCount]*list.List             // tried addrs.

	started  int32
	shutdown int32
	wg       sync.WaitGroup
	quit     chan struct{}
	nTried   int // trired addrs number.
	nNew     int // new addrs number.

	lamtx          sync.Mutex
	localAddresses map[string]*localAddress
	version        int

	log log.Logger
}

type localAddress struct {
	Addr net.Addr
	Port uint16
}

// loadPeers loads the known address from the saved file.
func (am *AddrManager) loadPeers() {
	am.mtx.Lock()
	defer am.mtx.Unlock()

	err := am.deserializePeers(am.peersFile)
	if err != nil {
		am.log.Errorln("Failed to parse file %s: %v", am.peersFile, err)
		// if it is invalid we nuke the old one unconditionally.
		err = os.Remove(am.peersFile)
		if err != nil {
			am.log.Warnln("Failed to remove corrupt peers file %s: %v",
				am.peersFile, err)
		}
		am.reset()
		return
	}
	am.log.Infoln("Loaded %d addresses from file '%s'", am.numAddresses(), am.peersFile)
}

// TODO
// 1. instance serializedAddrManager
// 2. check version and get key
// 3. serializedKnownAddress -> KnownAddress and store addrIndex
// 4. filled new or tried buckets using buckets in serializedAddrManager as key
func (am *AddrManager) deserializePeers(filePath string) error {
	return nil
}

func (am *AddrManager) reset() {
	am.addrIndex = make(map[string]*KnownAddress)

	// fill key with bytes from a good random source.
	io.ReadFull(crand.Reader, am.key[:])
	for i := range am.addrNew {
		am.addrNew[i] = make(map[string]*KnownAddress)
	}
	for i := range am.addrTried {
		am.addrTried[i] = list.New()
	}
}

func (am *AddrManager) numAddresses() int {
	return am.nTried + am.nNew
}

func NetAddressKey(na *protocol.NetAddress) string {
	port := strconv.FormatUint(uint64(na.Port), 10)

	return net.JoinHostPort(na.Addr.String(), port)
}

func (am *AddrManager) addressHandler() {
	dumpAddressTicker := time.NewTicker(dumpAddressInterval)
	defer dumpAddressTicker.Stop()
out:
	for {
		select {
		case <-dumpAddressTicker.C:
			am.savePeers()

		case <-am.quit:
			break out
		}
	}
	am.savePeers()
	am.wg.Done()
	am.log.Traceln("Address handler done")
}

// TODO
func (am *AddrManager) savePeers() {
	/* 	sam := new(serializedAddrManager)
	   	i := 0
	   	for k, v := range am.addrIndex {
	   		//sam.Addresses[i] =
	   		i++
	   	}
	   	for i := range am.addrNew {
	   		j := 0
	   		for k := range am.addrNew[i] {
	   			sam.NewBuckets[i][j] = k
	   		}
	   		j++
	   	}
	   	for i := range am.addrTried {
	   	} */
}

// peer addr msg will update am addresses set.
func (am *AddrManager) AddAddresses(addrs []*localAddress, srcAddr *localAddress) {
	am.mtx.Lock()
	defer am.mtx.Unlock()

	for _, na := range addrs {
		am.updateAddress(na, srcAddr)
	}
}

// todo
// 1. IsRoutable
// 2. check in sets and timestamp, update KnownAddress.
// 3. check TriedBucket and NewBucket
// 4. get NewBucket index
// 5. add new addr to NewBucket （index depend on key+srcAddr）
func (am *AddrManager) updateAddress(netAddr, srcAddr *localAddress) {
}

// todo
// using for after peer choose addr and establish connection.
func (am *AddrManager) NewToTriedBucket(addr *localAddress) {
}

// todo
// random select an peer for connection.
func (am *AddrManager) GetAddress() *KnownAddress {
	return nil
}

func (am *AddrManager) Start() {
	if atomic.AddInt32(&am.started, 1) != 1 {
		return
	}
	am.log.Traceln("Starting address manager")
	// Load peers we already know about from file.
	am.loadPeers()
	// Start the address ticker to save addresses periodically.
	am.wg.Add(1)
	go am.addressHandler()
}

func New(dataPath string, lookupFunc func(string) ([]net.IP, error)) *AddrManager {
	am := AddrManager{
		peersFile:      filepath.Join(dataPath, "peers.json"),
		lookupFunc:     lookupFunc,
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
		quit:           make(chan struct{}),
		localAddresses: make(map[string]*localAddress),
	}
	am.reset()
	return &am
}
