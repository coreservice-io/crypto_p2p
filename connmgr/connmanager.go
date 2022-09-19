package connmgr

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/log"
)

// maxFailedAttempts is the maximum number of successive failed connection
// attempts after which network failure is assumed and new connections will
// be delayed by the configured retry duration.
const maxFailedAttempts = 25

var (

	// maxRetryDuration is the max duration of time retrying of a persistent
	// connection is allowed to grow to.  This is necessary since the retry
	// logic uses a backoff mechanism which increases the interval base times
	// the number of retries that have been done.
	maxRetryDuration = time.Minute * 5

	// defaultRetryDuration is the default duration of time for retrying
	// persistent connections.
	defaultRetryDuration = time.Second * 5

	// defaultTargetOutbound is the default number of outbound connections to
	// maintain.
	defaultTargetOutbound = uint32(8)
)

// the state of the requested connection.
type ConnState uint8

const (
	ConnPending ConnState = iota
	ConnFailing
	ConnCanceled
	ConnEstablished
	ConnDisconnected
)

// connection request to a network address.
// If permanent, will be retried on disconnection.
type ConnReq struct {
	// atomically.
	id uint64 // seq for index.

	Addr      net.Addr // dest address.
	Permanent bool     // keep-alive.

	conn       net.Conn     // connection object.
	state      ConnState    // five connction states.
	stateMtx   sync.RWMutex // protect state rwlock.
	retryCount uint32       // record retry times if keep-alive.
}

// updates the state.
func (c *ConnReq) updateState(state ConnState) {
	c.stateMtx.Lock()
	c.state = state
	c.stateMtx.Unlock()
}

// get the unique identifier.
func (c *ConnReq) ID() uint64 {
	return atomic.LoadUint64(&c.id)
}

// get the connection state.
func (c *ConnReq) State() ConnState {
	c.stateMtx.RLock()
	state := c.state
	c.stateMtx.RUnlock()
	return state
}

// get the string id.
func (c *ConnReq) String() string {
	if c.Addr == nil || c.Addr.String() == "" {
		return fmt.Sprintf("reqid %d", atomic.LoadUint64(&c.id))
	}
	return fmt.Sprintf("%s (reqid %d)", c.Addr, atomic.LoadUint64(&c.id))
}

// config options for the connection manager.
type Config struct {
	// take ownership of and accept connections.
	Listeners []net.Listener

	// callback when an inbound connection is accepted.
	OnAccept func(net.Conn)

	// outbound network connections, Defaults to 8.
	TargetOutbound uint32

	// duration to wait before retrying, Defaults to 5s, max to 5min.
	RetryDuration time.Duration

	// callback when a new outbound connection is established.
	OnConnection func(*ConnReq, net.Conn)

	// callback when an outbound connection is disconnected.
	OnDisconnection func(*ConnReq)

	// get an new address to make a network connection to if connection failed.
	GetNewAddress func() (net.Addr, error)

	// connects to the address.
	Dial func(net.Addr) (net.Conn, error)
}

// register a pending connection attempt.
// allow callers to cancel pending.
type registerPending struct {
	c    *ConnReq
	done chan struct{}
}

// queue a successful connection.
type handleConnected struct {
	c    *ConnReq
	conn net.Conn
}

// remove a connection.
type handleDisconnected struct {
	id    uint64
	retry bool
}

// remove a pending connection.
type handleFailed struct {
	c   *ConnReq
	err error
}

// a manager to handle network connections.
type ConnManager struct {
	connReqCount uint64 // outbound number.
	start        int32
	stop         int32

	cfg            Config
	wg             sync.WaitGroup   // sync with cm exit status, blocking wait for goroutine exit.
	failedAttempts uint64           // retry times connecting to new peers once connection failed.
	requests       chan interface{} // communicate with goroutine.
	quit           chan struct{}    // notify goroutine exit.

	log log.Logger
}

// for permanent, reconnect after timer duration.
// otherwise, NewConnReq and for maxFailed retried after timer duration.
func (cm *ConnManager) handleFailedConn(c *ConnReq) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	if c.Permanent {
		c.retryCount++
		d := time.Duration(c.retryCount) * cm.cfg.RetryDuration
		if d > maxRetryDuration {
			d = maxRetryDuration
		}
		cm.log.Debugln("Retrying connection to %v in %v", c, d)
		time.AfterFunc(d, func() {
			cm.Connect(c)
		})
	} else if cm.cfg.GetNewAddress != nil {
		cm.failedAttempts++
		if cm.failedAttempts >= maxFailedAttempts {
			cm.log.Debugln("Max failed connection attempts reached: [%d] "+
				"-- retrying connection in: %v", maxFailedAttempts,
				cm.cfg.RetryDuration)
			time.AfterFunc(cm.cfg.RetryDuration, func() {
				cm.NewConnReq()
			})
		} else {
			go cm.NewConnReq()
		}
	}
}

// handles all connection related requests: connected or disconnected.
//
// maintain a pool of active outbound connections.
// Connection requests are processed and mapped by their assigned ids.
func (cm *ConnManager) connHandler() {

	var (
		// holds all registered conn requests.
		pending = make(map[uint64]*ConnReq)

		// the set of all actively outbound connected peers.
		conns = make(map[uint64]*ConnReq, cm.cfg.TargetOutbound)
	)

out:
	for {
		select {
		case req := <-cm.requests:
			switch msg := req.(type) {

			case registerPending:
				connReq := msg.c
				connReq.updateState(ConnPending)
				pending[msg.c.id] = connReq
				close(msg.done)
				// connection success:
			// 1. update state to ConnEstablished.
			// 2. add to conns map.
			// 3. reset retryCount and failedAttempts.
			// 4. callback OnConnection at new goroutine.
			case handleConnected:
				connReq := msg.c

				if _, ok := pending[connReq.id]; !ok {
					if msg.conn != nil {
						msg.conn.Close()
					}
					cm.log.Debugln("Ignoring connection for "+
						"canceled connreq=%v", connReq)
					continue
				}

				connReq.updateState(ConnEstablished)
				connReq.conn = msg.conn
				conns[connReq.id] = connReq
				cm.log.Debugln("Connected to %v", connReq)
				connReq.retryCount = 0
				cm.failedAttempts = 0

				delete(pending, connReq.id)

				if cm.cfg.OnConnection != nil {
					go cm.cfg.OnConnection(connReq, msg.conn)
				}
				// connection disconected:
			// 1. find req in map and update state to ConnCanceled.
			// 2. close tcp connection.
			// 3. callback OnDisconnection at new goroutine.
			// 4. if need, reconnect or choose new peer.
			case handleDisconnected:
				connReq, ok := conns[msg.id]
				if !ok {
					connReq, ok = pending[msg.id]
					if !ok {
						cm.log.Errorln("Unknown connid=%d",
							msg.id)
						continue
					}

					connReq.updateState(ConnCanceled)
					cm.log.Debugln("Canceling: %v", connReq)
					delete(pending, msg.id)
					continue

				}
				// remove it from conns map
				cm.log.Debugln("Disconnected from %v", connReq)
				delete(conns, msg.id)

				if connReq.conn != nil {
					connReq.conn.Close()
				}

				if cm.cfg.OnDisconnection != nil {
					go cm.cfg.OnDisconnection(connReq)
				}

				// after clean up state, no further attempts with this request.
				if !msg.retry {
					connReq.updateState(ConnDisconnected)
					continue
				}

				// when need more peersor persistent, re added to the
				// pending map for trying to reconect.
				if uint32(len(conns)) < cm.cfg.TargetOutbound ||
					connReq.Permanent {

					connReq.updateState(ConnPending)
					cm.log.Debugln("Reconnecting to %v",
						connReq)
					pending[msg.id] = connReq
					cm.handleFailedConn(connReq)
				}
				// conection failed:
			// 1. update state to ConnFailing.
			// 2. reconnect or choose new peer.
			case handleFailed:
				connReq := msg.c

				if _, ok := pending[connReq.id]; !ok {
					cm.log.Debugln("Ignoring connection for "+
						"canceled conn req: %v", connReq)
					continue
				}

				connReq.updateState(ConnFailing)
				cm.log.Debugln("Failed to connect to %v: %v",
					connReq, msg.err)
				cm.handleFailedConn(connReq)
			}

		case <-cm.quit:
			break out
		}
	}

	cm.wg.Done()
	cm.log.Traceln("Connection handler done")
}

// creates a new connection request.
// connects to the corresponding address.
func (cm *ConnManager) NewConnReq() {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	if cm.cfg.GetNewAddress == nil {
		return
	}

	c := &ConnReq{}
	atomic.StoreUint64(&c.id, atomic.AddUint64(&cm.connReqCount, 1))

	// Register pending connection attempt.
	done := make(chan struct{})
	select {
	case cm.requests <- registerPending{c, done}:
	case <-cm.quit:
		return
	}

	// Wait for the registration to successfully and pending conn req.
	select {
	case <-done:
	case <-cm.quit:
		return
	}

	addr, err := cm.cfg.GetNewAddress()
	if err != nil {
		select {
		case cm.requests <- handleFailed{c, err}:
		case <-cm.quit:
		}
		return
	}

	c.Addr = addr

	// connect with the id + addr
	cm.Connect(c)
}

// assigns an id.
// dials a connection to addr.
func (cm *ConnManager) Connect(c *ConnReq) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}

	// connState was cancelled during the time we wait for retry.
	if c.State() == ConnCanceled {
		cm.log.Debugln("Ignoring connect for canceled connreq=%v", c)
		return
	}

	if atomic.LoadUint64(&c.id) == 0 {
		atomic.StoreUint64(&c.id, atomic.AddUint64(&cm.connReqCount, 1))

		done := make(chan struct{})
		select {
		case cm.requests <- registerPending{c, done}:
		case <-cm.quit:
			return
		}

		select {
		case <-done:
		case <-cm.quit:
			return
		}
	}

	cm.log.Debugln("Attempting to connect to %v", c)

	conn, err := cm.cfg.Dial(c.Addr)
	if err != nil {
		select {
		case cm.requests <- handleFailed{c, err}:
		case <-cm.quit:
		}
		return
	}

	select {
	case cm.requests <- handleConnected{c, conn}:
	case <-cm.quit:
	}
}

// disconnects the connection with id.
// If permanent, the connection will be retried.
func (cm *ConnManager) Disconnect(id uint64) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}

	select {
	case cm.requests <- handleDisconnected{id, true}:
	case <-cm.quit:
	}
}

// removes the connection with id.
func (cm *ConnManager) Remove(id uint64) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}

	select {
	case cm.requests <- handleDisconnected{id, false}:
	case <-cm.quit:
	}
}

// listenHandler accepts incoming connections on a given listener.
func (cm *ConnManager) listenHandler(listener net.Listener) {
	cm.log.Infoln("Server listening on %s", listener.Addr())
	for atomic.LoadInt32(&cm.stop) == 0 {
		conn, err := listener.Accept()
		if err != nil {
			if atomic.LoadInt32(&cm.stop) == 0 {
				cm.log.Errorln("Can't accept connection: %v", err)
			}
			continue
		}
		go cm.cfg.OnAccept(conn)
	}

	cm.wg.Done()
	cm.log.Traceln("Listener handler done for %s", listener.Addr())
}

// launches the connection manager.
// begins connecting to the network.
func (cm *ConnManager) Start() {
	if atomic.AddInt32(&cm.start, 1) != 1 {
		return
	}

	cm.log.Traceln("Connection manager started")
	cm.wg.Add(1)
	// 1. start work goroutine.
	go cm.connHandler()

	if cm.cfg.OnAccept != nil {
		for _, listner := range cm.cfg.Listeners {
			cm.wg.Add(1)
			// 2. start listen goroutine waiting for peers connection.
			go cm.listenHandler(listner)
		}
	}

	for i := atomic.LoadUint64(&cm.connReqCount); i < uint64(cm.cfg.TargetOutbound); i++ {
		// 3. start establishing connection goroutine, choose peer and make connection.
		go cm.NewConnReq()
	}
}

// blocks until the connection manager halts gracefully.
func (cm *ConnManager) Wait() {
	cm.wg.Wait()
}

// gracefully shuts down the connection manager.
func (cm *ConnManager) Stop() {
	if atomic.AddInt32(&cm.stop, 1) != 1 {
		cm.log.Warnln("Connection manager already stopped")
		return
	}

	// Stop all the listeners.
	for _, listener := range cm.cfg.Listeners {
		_ = listener.Close()
	}

	close(cm.quit)
	cm.log.Traceln("Connection manager stopped")
}

// returns a new connection manager.
func New(cfg *Config) (*ConnManager, error) {
	if cfg.Dial == nil {
		return nil, errors.New("Config: Dial cannot be nil")
	}
	if cfg.RetryDuration <= 0 {
		cfg.RetryDuration = defaultRetryDuration
	}
	if cfg.TargetOutbound == 0 {
		cfg.TargetOutbound = defaultTargetOutbound
	}
	cm := ConnManager{
		cfg:      *cfg,
		requests: make(chan interface{}),
		quit:     make(chan struct{}),
	}
	return &cm, nil
}
