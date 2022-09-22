package addrmgr

import (
	"container/list"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	newBucketCount       = 1024
	triedBucketCount     = 8
	dumpAddressInterval  = time.Minute * 10
	serialisationVersion = 2
	newBucketsPerAddress = 8
	newBucketsPerGroup   = 64
	newBucketSize        = 64
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
	na          *protocol.NetAddress
	srcAddr     *protocol.NetAddress
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

// 1. instance serializedAddrManager
// 2. check version and get key
// 3. serializedKnownAddress -> KnownAddress and store addrIndex
// 4. filled new or tried buckets using buckets in serializedAddrManager as key
func (am *AddrManager) deserializePeers(filePath string) error {

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	r, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("%s error opening file: %v", filePath, err)
	}
	defer r.Close()

	var sam serializedAddrManager
	dec := json.NewDecoder(r)
	err = dec.Decode(&sam)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", filePath, err)
	}

	if sam.Version > serialisationVersion {
		return fmt.Errorf("unknown version %v in serialized "+
			"addrmanager", sam.Version)
	}

	copy(am.key[:], sam.Key[:])

	for _, v := range sam.Addresses {
		ka := new(KnownAddress)

		ka.na, err = am.DeserializeNetAddress(v.Addr)
		if err != nil {
			return fmt.Errorf("failed to deserialize netaddress "+
				"%s: %v", v.Addr, err)
		}

		ka.srcAddr, err = am.DeserializeNetAddress(v.Src)
		if err != nil {
			return fmt.Errorf("failed to deserialize netaddress "+
				"%s: %v", v.Src, err)
		}

		ka.attempts = v.Attempts
		ka.lastattempt = time.Unix(v.LastAttempt, 0)
		ka.lastsuccess = time.Unix(v.LastSuccess, 0)
		am.addrIndex[NetAddressKey(ka.na)] = ka
	}

	for i := range sam.NewBuckets {
		for _, val := range sam.NewBuckets[i] {
			ka, ok := am.addrIndex[val]
			if !ok {
				return fmt.Errorf("newbucket contains %s but "+
					"none in address list", val)
			}

			if ka.refs == 0 {
				am.nNew++
			}
			ka.refs++
			am.addrNew[i][val] = ka
		}
	}
	for i := range sam.TriedBuckets {
		for _, val := range sam.TriedBuckets[i] {
			ka, ok := am.addrIndex[val]
			if !ok {
				return fmt.Errorf("Newbucket contains %s but "+
					"none in address list", val)
			}

			ka.tried = true
			am.nTried++
			am.addrTried[i].PushBack(ka)
		}
	}

	// ensure address with ref or tried.
	for k, v := range am.addrIndex {
		if v.refs == 0 && !v.tried {
			return fmt.Errorf("address %s after serialisation "+
				"with no references", k)
		}

		if v.refs > 0 && v.tried {
			return fmt.Errorf("address %s after serialisation "+
				"which is both new and tried!", k)
		}
	}

	return nil
}

func (am *AddrManager) DeserializeNetAddress(addr string) (*protocol.NetAddress, error) {

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	return am.HostToNetAddress(host, uint16(port))
}

func (am *AddrManager) HostToNetAddress(host string, port uint16) (*protocol.NetAddress, error) {

	var (
		na      *protocol.NetAddress
		ip      net.IP
		netAddr net.Addr
	)

	if ip = net.ParseIP(host); ip == nil {
		ips, err := am.lookupFunc(host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for %s", host)
		}
		ip = ips[0]
	}

	addr := &ipv4Addr{}
	addr.netID = 1
	copy(addr.addr[:], ip)
	netAddr = addr

	na = &protocol.NetAddress{
		Addr: netAddr,
		Port: port,
	}

	return na, nil
}

type ipv4Addr struct {
	addr  [4]byte
	netID uint8
}

func (a *ipv4Addr) String() string {
	return net.IP(a.addr[:]).String()
}

func (a *ipv4Addr) Network() string {
	return string(a.netID)
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

func (am *AddrManager) savePeers() {
	am.mtx.Lock()
	defer am.mtx.Unlock()

	sam := new(serializedAddrManager)
	sam.Version = am.version
	copy(sam.Key[:], am.key[:])

	sam.Addresses = make([]*serializedKnownAddress, len(am.addrIndex))
	i := 0
	for k, v := range am.addrIndex {
		ska := new(serializedKnownAddress)
		ska.Addr = k

		ska.Src = NetAddressKey(v.srcAddr)
		ska.Attempts = v.attempts
		ska.LastAttempt = v.lastattempt.Unix()
		ska.LastSuccess = v.lastsuccess.Unix()

		sam.Addresses[i] = ska
		i++
	}
	for i := range am.addrNew {
		sam.NewBuckets[i] = make([]string, len(am.addrNew[i]))
		j := 0
		for k := range am.addrNew[i] {
			sam.NewBuckets[i][j] = k
			j++
		}
	}
	for i := range am.addrTried {
		sam.TriedBuckets[i] = make([]string, am.addrTried[i].Len())
		j := 0
		for e := am.addrTried[i].Front(); e != nil; e = e.Next() {
			ka := e.Value.(*KnownAddress)
			sam.TriedBuckets[i][j] = NetAddressKey(ka.na)
			j++
		}
	}

	w, err := os.Create(am.peersFile)
	if err != nil {
		am.log.Errorln("Error opening file %s: %v", am.peersFile, err)
		return
	}
	enc := json.NewEncoder(w)
	defer w.Close()
	if err := enc.Encode(&sam); err != nil {
		am.log.Errorln("Failed to encode file %s: %v", am.peersFile, err)
		return
	}
}

// peer addr msg will update am addresses set.
func (am *AddrManager) AddAddresses(addrs []*protocol.NetAddress, srcAddr *protocol.NetAddress) {
	am.mtx.Lock()
	defer am.mtx.Unlock()

	for _, na := range addrs {
		am.updateAddress(na, srcAddr)
	}
}

func (a *AddrManager) AddAddress(addr, srcAddr *protocol.NetAddress) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.updateAddress(addr, srcAddr)
}

// todo
// 1. IsRoutable
// 2. check in sets and timestamp, update KnownAddress.
// 3. check TriedBucket and NewBucket
// 4. get NewBucket index
// 5. add new addr to NewBucket （index depend on key+srcAddr）
func (am *AddrManager) updateAddress(netAddr, srcAddr *protocol.NetAddress) {

	addr := NetAddressKey(netAddr)
	ka := am.addrIndex[NetAddressKey(netAddr)]
	if ka != nil {
		if ka.tried {
			return
		}

		if ka.refs == newBucketsPerAddress {
			return
		}

		factor := int32(2 * ka.refs)
		if am.rand.Int31n(factor) != 0 {
			return
		}
	} else {
		netAddrCopy := *netAddr
		ka = &KnownAddress{na: &netAddrCopy, srcAddr: srcAddr}
		am.addrIndex[addr] = ka
		am.nNew++
	}

	bucket := am.getNewBucket(netAddr, srcAddr)

	if _, ok := am.addrNew[bucket][addr]; ok {
		return
	}

	// Enforce max addresses.
	if len(am.addrNew[bucket]) > newBucketSize {
		am.log.Traceln("new bucket is full, expiring old")
		// am.expireNew(bucket)
	}

	// Add to new bucket.
	ka.refs++
	am.addrNew[bucket][addr] = ka

	am.log.Traceln("Added new address %s for a total of %d addresses", addr,
		am.nTried+am.nNew)
}

func (a *AddrManager) getNewBucket(netAddr, srcAddr *protocol.NetAddress) int {
	// doublesha256(key + sourcegroup + int64(doublesha256(key + group + sourcegroup))
	// %bucket_per_source_group) % num_new_buckets

	data1 := []byte{}
	data1 = append(data1, a.key[:]...)
	data1 = append(data1, []byte(netAddr.Addr.String())...)
	data1 = append(data1, []byte(srcAddr.Addr.String())...)
	hash1 := doubleHashByte(data1)
	hash64 := binary.LittleEndian.Uint64(hash1)
	hash64 %= newBucketsPerGroup
	var hashbuf [8]byte
	binary.LittleEndian.PutUint64(hashbuf[:], hash64)
	data2 := []byte{}
	data2 = append(data2, a.key[:]...)
	data2 = append(data2, []byte(srcAddr.Addr.String())...)
	data2 = append(data2, hashbuf[:]...)

	hash2 := doubleHashByte(data2)
	return int(binary.LittleEndian.Uint64(hash2) % newBucketCount)
}

func doubleHashByte(data []byte) []byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:]
}

// todo!!!
// using for after peer choose addr and establish connection.
func (am *AddrManager) NewToTriedBucket(addr *localAddress) {
}

// todo !!!
// random select an peer for connection.
func (am *AddrManager) GetAddress() *KnownAddress {
	return nil
}

// todo !!!
func (a *AddrManager) AddressCache() []*protocol.NetAddress {
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
