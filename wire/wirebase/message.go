package wirebase

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"
)

func caclCheckSum(payload []byte) []byte {

	// Test checksum.
	checksum := DoubleHashB(payload)[0:4]

	return checksum
}

// reads a message header from r.
func ReadMessageHeader(r io.Reader) (int, *messageHeader, error) {
	// Since readElements doesn't return the amount of bytes read, attempt
	// to read the entire header into a buffer first in case there is a
	// short read so the proper amount of read bytes are known.  This works
	// since the header is a fixed size.
	var headerBytes [MessageHeaderSize]byte
	n, err := io.ReadFull(r, headerBytes[:])
	if err != nil {
		return n, nil, err
	}
	hr := bytes.NewReader(headerBytes[:])

	// Create and populate a messageHeader struct from the raw header bytes.
	hdr := messageHeader{}
	var command [CommandSize]byte
	readElements(hr, &hdr.magic, &command, &hdr.length, &hdr.checksum)

	// Strip trailing zeros from command string.
	hdr.command = string(bytes.TrimRight(command[:], "\x00"))

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

// writes a Message to w including the necessary header information.
func WriteMessage(w io.Writer, msg Message, pver uint32, netMagic NetMagic) error {
	_, err := WriteMessageWithEncodingN(w, msg, pver, netMagic, BaseEncoding)
	return err
}

// writes a Message to w including the necessary header information
// and returns the number of bytes written.
func WriteMessageN(w io.Writer, msg Message, pver uint32, netMagic NetMagic) (int, error) {
	return WriteMessageWithEncodingN(w, msg, pver, netMagic, BaseEncoding)
}

// writes a Message to w including the necessary header information
// and returns the number of bytes written.
func WriteMessageWithEncodingN(w io.Writer, msg Message, pver uint32,
	netMagic NetMagic, enc MessageEncoding) (int, error) {

	totalBytes := 0

	// Enforce max command size.
	var command [CommandSize]byte
	cmd := msg.Command()
	if len(cmd) > CommandSize {
		str := fmt.Sprintf("command [%s] is too long [max %v]",
			cmd, CommandSize)
		return totalBytes, NewMessageError("WriteMessage", str)
	}
	copy(command[:], []byte(cmd))

	// Encode the message payload.
	var bw bytes.Buffer
	err := msg.Encode(&bw, pver)
	if err != nil {
		return totalBytes, err
	}
	payload := bw.Bytes()
	lenp := len(payload)

	// Enforce maximum overall message payload.
	if lenp > MaxMessagePayload {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			lenp, MaxMessagePayload)
		return totalBytes, NewMessageError("WriteMessage", str)
	}

	// Enforce maximum message payload based on the message type.
	mpl := msg.MaxPayloadLength(pver)
	if uint32(lenp) > mpl {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload size for "+
			"messages of type [%s] is %d.", lenp, cmd, mpl)
		return totalBytes, NewMessageError("WriteMessage", str)
	}

	// Create header for the message.
	var checksum [4]byte
	copy(checksum[:], caclCheckSum(payload))

	// Encode the header for the message.  This is done to a buffer
	// rather than directly to the writer since writeElements doesn't
	// return the number of bytes written.
	hw := bytes.NewBuffer(make([]byte, 0, MessageHeaderSize))
	writeElements(hw, uint32(netMagic), command, uint32(lenp), checksum)

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

// reads, validates, and parses the next Message from r for
// the provided protocol version and network.
// It returns the parsed Message and raw bytes which comprise the message.
func ReadMessage(r io.Reader, pver uint32, netMagic NetMagic, makeEmptyMsgFunc MakeEmptyMessage) (Message, []byte, error) {
	_, msg, buf, err := ReadMessageWithEncodingN(r, pver, netMagic, makeEmptyMsgFunc, BaseEncoding)
	return msg, buf, err
}

// reads, validates, and parses the next Message from r for
// the provided protocol version and network.
// It returns the number of bytes read in addition to the parsed Message
// and raw bytes which comprise the message.
func ReadMessageN(r io.Reader, pver uint32, netMagic NetMagic, makeEmptyMsgFunc MakeEmptyMessage) (int, Message, []byte, error) {
	return ReadMessageWithEncodingN(r, pver, netMagic, makeEmptyMsgFunc, BaseEncoding)
}

// reads, validates, and parses the next Message
// from r for the provided protocol version and network.
// It returns the number of bytes read in addition to the parsed Message
// and raw bytes which comprise the message.
func ReadMessageWithEncodingN(r io.Reader, pver uint32,
	netMagic NetMagic, makeEmptyMsgFunc MakeEmptyMessage, enc MessageEncoding) (int, Message, []byte, error) {

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
	if hdr.length > MaxMessagePayload {
		str := fmt.Sprintf("message payload is too large - header "+
			"indicates %d bytes, but max message payload is %d "+
			"bytes.", hdr.length, MaxMessagePayload)
		return totalBytes, nil, nil, NewMessageError("ReadMessage", str)
	}

	// Check for malformed commands.
	command := hdr.command
	if !utf8.ValidString(command) {
		discardInput(r, hdr.length)
		str := fmt.Sprintf("invalid command %v", []byte(command))
		return totalBytes, nil, nil, NewMessageError("ReadMessage", str)
	}

	// Create struct of appropriate message type based on the command.
	msg, err := makeEmptyMsgFunc(command)
	if err != nil {
		// makeEmptyMessage can only return ErrUnknownMessage and it is
		// important that we bubble it up to the caller.
		discardInput(r, hdr.length)
		return totalBytes, nil, nil, err
	}

	// Check for maximum length based on the message type as a malicious client
	// could otherwise create a well-formed header and set the length to max
	// numbers in order to exhaust the machine's memory.
	mpl := msg.MaxPayloadLength(pver)
	if hdr.length > mpl {
		discardInput(r, hdr.length)
		str := fmt.Sprintf("payload exceeds max length - header "+
			"indicates %v bytes, but max payload size for messages of type [%v] is %v.",
			hdr.length, command, mpl)
		return totalBytes, nil, nil, NewMessageError("ReadMessage", str)
	}

	// Read payload.
	payload := make([]byte, hdr.length)
	n, err = io.ReadFull(r, payload)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Test checksum.
	checksum := caclCheckSum(payload)
	if !bytes.Equal(checksum, hdr.checksum[:]) {
		str := fmt.Sprintf("payload checksum failed - header "+
			"indicates %v, but actual checksum is %v.",
			hdr.checksum, checksum)
		return totalBytes, nil, nil, NewMessageError("ReadMessage", str)
	}

	// Unmarshal message.
	// NOTE: This must be a *bytes.Buffer since the MsgVersion Decode function requires it.
	pr := bytes.NewBuffer(payload)
	err = msg.Decode(pr, pver)
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return totalBytes, msg, payload, nil
}
