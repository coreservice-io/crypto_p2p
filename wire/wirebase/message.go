package wirebase

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"sync"
)

// reads a message header.
func ReadMessageHeader(r io.Reader) (int, *messageHeader, error) {
	// Since readElements doesn't return the amount of bytes read,
	// attempt to read the entire header into a buffer first in case there is a
	// short read so the proper amount of read bytes are known.
	// This works since the header is a fixed size.
	var headerBytes [MSG_HEADER_SIZE]byte
	n, err := io.ReadFull(r, headerBytes[:])
	if err != nil {
		return n, nil, err
	}
	hr := bytes.NewReader(headerBytes[:])

	// Create and populate a messageHeader struct from the raw header bytes.
	hdr := messageHeader{}
	readElements(hr, &hdr.magic, &hdr.id, &hdr.command, &hdr.length,
		&hdr.ck_size, &hdr.ck_idx,
	)

	return n, &hdr, nil
}

// reads n bytes from reader r in chunks and discards the read bytes.
// This is used to skip payloads when various errors occur and helps
// prevent rogue nodes from causing massive memory allocation through forging
// header length.
func discardInput(r io.Reader, n uint32) {
	maxSize := uint32(10 * 1024) // 10k at a time
	numReads := n / maxSize
	bytesRemaining := n % maxSize
	if n > 0 {
		buf := make([]byte, maxSize)
		for i := uint32(0); i < numReads; i++ {
			io.ReadFull(r, buf)
		}
	}
	if bytesRemaining > 0 {
		buf := make([]byte, bytesRemaining)
		io.ReadFull(r, buf)
	}
}

// writes a Message to w including the necessary header information
// and returns the number of bytes written.
func WriteMessage(w io.Writer, msg Message, pver uint32, netMagic NetMagic) (int, error) {
	// Encode the message payload.
	var bw bytes.Buffer
	err := msg.Encode(&bw, pver)
	if err != nil {
		return 0, err
	}
	payload := bw.Bytes()

	hdr := messageHeader{
		magic:   uint32(netMagic),
		id:      rand.Uint32(),
		command: msg.Command(),
		ck_size: 1,
		ck_idx:  1,
	}

	return WritePayload(w, hdr, payload)
}

// reads, validates, and parses the next Message
// It returns the number of bytes read in addition to the parsed Message
// and raw bytes which comprise the message.
func ReadMessage(r io.Reader, longMessages sync.Map,
	pver uint32, netMagic NetMagic, makeEmptyMsgFunc MakeEmptyMessage) (int, Message, []byte, error) {

	for {
		totalBytes, msg, payload, err := ReadPayload(r, longMessages, netMagic, makeEmptyMsgFunc)
		if err != nil {
			return totalBytes, nil, nil, err
		}
		if msg != nil {
			// Decode message.
			pr := bytes.NewBuffer(payload)
			err = msg.Decode(pr, pver)
			if err != nil {
				return totalBytes, nil, nil, err
			}

			return totalBytes, msg, payload, nil
		}
	}
}

func WritePayload(w io.Writer, hdr messageHeader, payload []byte) (int, error) {

	lenp := len(payload)

	// Enforce maximum overall message payload.
	if lenp > MSG_MAX_PAYLOAD_SIZE {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			lenp, MSG_MAX_PAYLOAD_SIZE)
		return 0, NewMessageError("WriteMessage", str)
	}

	// Create header for the message.

	// Encode the header for the message.  This is done to a buffer
	// rather than directly to the writer since writeElements doesn't
	// return the number of bytes written.
	hw := bytes.NewBuffer(make([]byte, 0, MSG_HEADER_SIZE))
	writeElements(hw, hdr.magic, hdr.id, hdr.command, uint32(lenp),
		hdr.ck_size, hdr.ck_idx)

	totalBytes := 0

	// Write header.
	n, err := w.Write(hw.Bytes())
	totalBytes += n
	if err != nil {
		return totalBytes, err
	}

	// Only write the payload if there is one,
	// e.g., verack messages don't have one.
	if len(payload) > 0 {
		n, err = w.Write(payload)
		totalBytes += n
	}

	return totalBytes, err
}

func ReadPayload(r io.Reader, longMessages sync.Map,
	netMagic NetMagic, makeEmptyMsgFunc MakeEmptyMessage) (int, Message, []byte, error) {

	totalBytes := 0
	n, hdr, err := ReadMessageHeader(r)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Check for messages from the wrong network.
	if hdr.magic != uint32(netMagic) {
		discardInput(r, hdr.length) // TODO: throw hold receive
		str := fmt.Sprintf("message from other network [%v]", hdr.magic)
		return totalBytes, nil, nil, NewMessageError("ReadMessage", str)
	}

	// Enforce maximum message payload.
	if hdr.length > MSG_MAX_PAYLOAD_SIZE {
		str := fmt.Sprintf("message payload is too large - header "+
			"indicates %d bytes, but max message payload is %d bytes.",
			hdr.length, MSG_MAX_PAYLOAD_SIZE)
		return totalBytes, nil, nil, NewMessageError("ReadMessage", str)
	}

	if hdr.ck_size > 1 {

		// Read payload.
		payload := make([]byte, hdr.length)
		n, err = io.ReadFull(r, payload)
		totalBytes += n
		if err != nil {
			return totalBytes, nil, nil, err
		}

		if hdr.ck_size >= hdr.ck_idx {
			longBuf, exists := longMessages.Load(hdr.id)
			if !exists {
				longBuf = bytes.NewBuffer(payload)
				longMessages.Store(hdr.id, longBuf)
			} else {
				longBuf.(*bytes.Buffer).Write(payload)
			}
			return totalBytes, nil, payload, nil
		}

		// last one
		if hdr.ck_size == hdr.ck_idx {

			// Create struct of appropriate message type based on the command.
			msg, err := makeEmptyMsgFunc(hdr.command)
			if err != nil {
				// makeEmptyMessage can only return ErrUnknownMessage and it is
				// important that we bubble it up to the caller.
				discardInput(r, hdr.length)
				return totalBytes, nil, nil, err
			}

			longBuf, exists := longMessages.Load(hdr.id)
			if !exists {
				longBuf = bytes.NewBuffer(payload)
				longMessages.Store(hdr.id, longBuf)
			} else {
				longBuf.(*bytes.Buffer).Write(payload)
			}

			return totalBytes, msg, longBuf.(*bytes.Buffer).Bytes(), nil
		}
	}

	// Create struct of appropriate message type based on the command.
	msg, err := makeEmptyMsgFunc(hdr.command)
	if err != nil {
		// makeEmptyMessage can only return ErrUnknownMessage and it is
		// important that we bubble it up to the caller.
		discardInput(r, hdr.length)
		return totalBytes, nil, nil, err
	}

	// Read payload.
	payload := make([]byte, hdr.length)
	n, err = io.ReadFull(r, payload)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return totalBytes, msg, payload, nil
}
