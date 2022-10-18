package peer

import (
	"container/list"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"
)

// outMsg is used to house a message to be sent along with a channel to signal
// when the message has been sent (or won't be sent due to things such as
// shutdown)
type outMsg struct {
	msg      wirebase.Message
	doneChan chan<- struct{}
}

// stallControlCmd represents the command of a stall control message.
type stallControlCmd uint8

// Constants for the command of a stall control message.
const (
	// sccSendMessage indicates a message is being sent to the remote peer.
	sccSendMessage stallControlCmd = iota

	// sccReceiveMessage indicates a message has been received from the
	// remote peer.
	sccReceiveMessage

	// sccHandlerStart indicates a callback handler is about to be invoked.
	sccHandlerStart

	// sccHandlerStart indicates a callback handler has completed.
	sccHandlerDone
)

// stallControlMsg is used to signal the stall handler about specific events
// so it can properly detect and handle stalled remote peers.
type stallControlMsg struct {
	command stallControlCmd
	message wirebase.Message
}

// sends a reject message for the provided command, reject code, reject reason, and hash.
// The wait parameter will cause the function to block
// until the reject message has actually been sent.
func (p *Peer) PushRejectMsg(command uint32, code wmsg.RejectCode, reason string, wait bool) {

	msg := wmsg.NewMsgReject(command, code, reason)

	// Send the message without waiting if the caller has not requested it.
	if !wait {
		p.QueueMessage(msg, nil)
		return
	}

	// Send the message and block until it has been sent before returning.
	doneChan := make(chan struct{}, 1)
	p.QueueMessage(msg, doneChan)
	<-doneChan
}

func (p *Peer) isAllowedReadError(err error) bool {
	// Don't allow the error if it's not specifically a malformed message error.
	if _, ok := err.(*wirebase.MessageError); !ok {
		return false
	}

	host, _, err := net.SplitHostPort(p.literalAddr)
	if err != nil {
		return false
	}

	if host != "127.0.0.1" && host != "localhost" {
		return false
	}

	return true
}

func (p *Peer) shouldHandleReadError(err error) bool {
	if atomic.LoadInt32(&p.connected) != 1 {
		return false
	}

	if err == io.EOF {
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
		return false
	}

	return true
}

// potentially adds a deadline for the appropriate expected
// response for the passed wire protocol command to the pending responses map.
func (p *Peer) maybeAddDeadline(pendingResponses map[uint32]time.Time, msgCmd uint32) {
	// Setup a deadline for each message being sent that expects a response.
	//
	// NOTE:
	// Pings are intentionally ignored here since they are typically
	// sent asynchronously and as a result of a long backlock of messages,
	// such as is typical in the case of initial block download, the
	// response won't be received in time.
	deadline := time.Now().Add(stallResponseTimeout)
	switch msgCmd {
	case msg.CMD_VERSION:
		// Expects a verack message.
		pendingResponses[msg.CMD_VERACK] = deadline
	}
}

// stallHandler handles stall detection for the peer.  This entails keeping
// track of expected responses and assigning them deadlines while accounting for
// the time spent in callbacks.  It must be run as a goroutine.
func (p *Peer) stallHandler() {
	// These variables are used to adjust the deadline times forward by the
	// time it takes callbacks to execute.  This is done because new
	// messages aren't read until the previous one is finished processing
	// (which includes callbacks), so the deadline for receiving a response
	// for a given message must account for the processing time as well.
	var handlerActive bool
	var handlersStartTime time.Time
	var deadlineOffset time.Duration

	// tracks the expected response deadline times.
	pendingResponses := make(map[uint32]time.Time)

	// stallTicker is used to periodically check pending responses that have
	// exceeded the expected deadline and disconnect the peer due to stalling.
	stallTicker := time.NewTicker(stallTickInterval)
	defer stallTicker.Stop()

	// ioStopped is used to detect when both the input and output handler
	// goroutines are done.
	var ioStopped bool
out:
	for {
		select {
		case msg := <-p.stallControl:

			switch msg.command {
			case sccSendMessage:
				// Add a deadline for the expected response message if needed.
				p.maybeAddDeadline(pendingResponses, msg.message.Command())

			case sccReceiveMessage:
				// Remove received messages from the expected response map.
				// Since certain commands expect one of a group of responses,
				// remove everything in the expected group accordingly.
				switch msgCmd := msg.message.Command(); msgCmd {

				default:
					delete(pendingResponses, msgCmd)
				}

			case sccHandlerStart:
				// Warn on unbalanced callback signalling.
				if handlerActive {
					log.Warn("Received handler start control command while a " +
						"handler is already active")
					continue
				}

				handlerActive = true
				handlersStartTime = time.Now()

			case sccHandlerDone:
				// Warn on unbalanced callback signalling.
				if !handlerActive {
					log.Warn("Received handler done control command when a " +
						"handler is not already active")
					continue
				}

				// Extend active deadlines by the time it took to execute the callback.
				duration := time.Since(handlersStartTime)
				deadlineOffset += duration
				handlerActive = false

			default:
				log.Warnf("Unsupported message command %v", msg.command)
			}

		case <-stallTicker.C:

			// Calculate the offset to apply to the deadline based on
			// how long the handlers have taken to execute since the last tick.
			now := time.Now()
			offset := deadlineOffset
			if handlerActive {
				offset += now.Sub(handlersStartTime)
			}

			for command, deadline := range pendingResponses {
				if now.Before(deadline.Add(offset)) {
					continue
				}

				log.Debugf("Peer %s appears to be stalled or misbehaving, %s timeout -- disconnecting",
					p, command)
				p.Close()
				break
			}

			// Reset the deadline offset for the next tick.
			deadlineOffset = 0

		case <-p.inQuit:
			// The stall handler can exit once both the input and
			// output handler goroutines are done.
			if ioStopped {
				break out
			}
			ioStopped = true

		case <-p.outQuit:
			// The stall handler can exit once both the input and
			// output handler goroutines are done.
			if ioStopped {
				break out
			}
			ioStopped = true
		}
	}

	// Drain any wait channels before going away so there is nothing left
	// waiting on this goroutine.
cleanup:
	for {
		select {
		case <-p.stallControl:
		default:
			break cleanup
		}
	}
	log.Tracef("Peer stall handler done for %s", p)
}

func (p *Peer) inHandler() {
	idleTimer := time.AfterFunc(idleTimeout, func() {
		log.Warnf("Peer %s no answer for %s -- disconnecting", p, idleTimeout)
		p.Close()
	})

	// message loop
out:
	for atomic.LoadInt32(&p.connected) != 1 {

		rmsg, err := p.readMessage()
		idleTimer.Stop()
		if err != nil {
			if p.isAllowedReadError(err) {
				log.Errorf("Allowed test error from %s: %v", p, err)
				idleTimer.Reset(idleTimeout)
				continue
			}

			if err == wmsg.ErrUnknownMessage {
				log.Debugf("Received unknown message from %s: %v", p, err)
				idleTimer.Reset(idleTimeout)
				continue
			}

			if p.shouldHandleReadError(err) {
				errMsg := fmt.Sprintf("Can't read message from %s: %v", p, err)
				if err != io.ErrUnexpectedEOF {
					log.Errorf(errMsg)
				}

				// p.PushRejectMsg(msg.CMD_REJECT, wmsg.RejectMalformed, errMsg, true)
			}
			break out
		}
		p.stallControl <- stallControlMsg{sccReceiveMessage, rmsg}

		// Handle each supported message type.
		p.stallControl <- stallControlMsg{sccHandlerStart, rmsg}
		switch message := rmsg.(type) {
		case *msg.MsgVersion:
			// Limit to one version message per peer.
			// p.PushRejectMsg(message.Command(), msg.RejectDuplicate,
			// 	"duplicate version message", true)
			break out

		case *msg.MsgVerAck:
			// Limit to one verack message per peer.
			// p.PushRejectMsg(
			// 	message.Command(), msg.RejectDuplicate,
			// 	"duplicate verack message", true,
			// )
			break out

		case *msg.MsgReject:

		default:
			if handler := p.peer_mgr.GetHandler(rmsg.Command()); handler != nil {
				handler(rmsg, p)
			} else {
				log.Debugf("Received unhandled message of type %v from %v",
					rmsg.Command(), p)
			}
		}
		p.stallControl <- stallControlMsg{sccHandlerDone, rmsg}

		// A message was received so reset the idle timer.
		idleTimer.Reset(idleTimeout)
	}

	// Ensure the idle timer is stopped to avoid leaking the resource.
	idleTimer.Stop()

	// Ensure connection is closed.
	p.Close()

	close(p.inQuit)
	log.Tracef("Peer input handler done for %s", p)
}

// handles the queuing of outgoing data for the peer.
// This runs as a muxer for various sources of input so we can ensure that
// server and peer handlers will not block on us sending a message.
// That data is then passed on to outHandler to be actually written.
func (p *Peer) queueHandler() {
	pendingMsgs := list.New()

	// We keep the waiting flag so that we know if we have a message queued
	// to the outHandler or not.  We could use the presence of a head of
	// the list for this but then we have rather racy concerns about whether
	// it has gotten it at cleanup time - and thus who sends on the
	// message's done channel.
	// To avoid such confusion we keep a different
	// flag and pendingMsgs only contains messages that we have not yet
	// passed to outHandler.
	waiting := false

	// To avoid duplication below.
	queuePacket := func(omsg outMsg, pendings *list.List, waiting bool) bool {
		if !waiting {
			p.sendQueue <- omsg
		} else {
			pendings.PushBack(omsg)
		}
		// we are always waiting now.
		return true
	}
out:
	for {
		select {
		case omsg := <-p.outputQueue:
			waiting = queuePacket(omsg, pendingMsgs, waiting)

		// This channel is notified when a message has been sent across the network socket.
		case <-p.sendDoneQueue:
			// No longer waiting if there are no more messages in the pending messages queue.
			next := pendingMsgs.Front()
			if next == nil {
				waiting = false
				continue
			}

			// Notify the outHandler about the next item to asynchronously send.
			val := pendingMsgs.Remove(next)
			p.sendQueue <- val.(outMsg)

		case <-p.quit:
			break out
		}
	}

	// Drain any wait channels before we go away so we don't leave something waiting for us.
	for e := pendingMsgs.Front(); e != nil; e = pendingMsgs.Front() {
		val := pendingMsgs.Remove(e)
		msg := val.(outMsg)
		if msg.doneChan != nil {
			msg.doneChan <- struct{}{}
		}
	}
cleanup:
	for {
		select {
		case msg := <-p.outputQueue:
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
		default:
			break cleanup
		}
	}
	close(p.queueQuit)
	log.Tracef("Peer queue handler done for %s", p)
}

// shouldLogWriteError returns whether or not the passed error,
// which is expected to have come from writing to the remote peer in the outHandler,
// should be logged.
func (p *Peer) shouldLogWriteError(err error) bool {
	// No logging when the peer is being forcibly disconnected.
	if atomic.LoadInt32(&p.connected) != 1 {
		return false
	}

	// No logging when the remote peer has been disconnected.
	if err == io.EOF {
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
		return false
	}

	return true
}

// outHandler handles all outgoing messages for the peer.
// It must be run as a goroutine.
// It uses a buffered channel to serialize output messages while
// allowing the sender to continue running asynchronously.
func (p *Peer) outHandler() {
out:
	for {
		select {
		case omsg := <-p.sendQueue:

			p.stallControl <- stallControlMsg{sccSendMessage, omsg.msg}

			err := p.writeMessage(omsg.msg)
			if err != nil {
				p.Close()
				if p.shouldLogWriteError(err) {
					log.Errorf("Failed to send message to %s: %v", p, err)
				}
				if omsg.doneChan != nil {
					omsg.doneChan <- struct{}{}
				}
				continue
			}

			// At this point, the message was successfully sent,
			// signal the sender of the message that it has been sent (if requested),
			// and signal the send queue to the deliver the next queued message.
			if omsg.doneChan != nil {
				omsg.doneChan <- struct{}{}
			}
			p.sendDoneQueue <- struct{}{}

		case <-p.quit:
			break out
		}
	}

	<-p.queueQuit

	// Drain any wait channels before we go away so we don't leave something
	// waiting for us. We have waited on queueQuit and thus we can be sure
	// that we will not miss anything sent on sendQueue.
cleanup:
	for {
		select {
		case omsg := <-p.sendQueue:
			if omsg.doneChan != nil {
				omsg.doneChan <- struct{}{}
			}
			// no need to send on sendDoneQueue since queueHandler
			// has been waited on and already exited.
		default:
			break cleanup
		}
	}
	close(p.outQuit)
	log.Tracef("Peer output handler done for %s", p)
}

// adds the passed message to the peer send queue.
func (p *Peer) QueueMessage(msg wirebase.Message, doneChan chan<- struct{}) {

	// Avoid risk of deadlock if goroutine already exited.
	// The goroutine we will be sending to hangs around until it knows for a fact that
	// it is marked as disconnected and *then* it drains the channels.
	if !p.Connected() {
		if doneChan != nil {
			go func() {
				doneChan <- struct{}{}
			}()
		}
		return
	}

	p.outputQueue <- outMsg{msg: msg, doneChan: doneChan}
}
