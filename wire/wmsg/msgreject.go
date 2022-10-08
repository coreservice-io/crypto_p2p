package wmsg

import (
	"fmt"
	"io"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

// RejectCode represents a numeric value by which a remote peer indicates
// why a message was rejected.
type RejectCode uint8

// These constants define the various supported reject codes.
const (
	RejectMalformed       RejectCode = 0x01
	RejectInvalid         RejectCode = 0x10
	RejectObsolete        RejectCode = 0x11
	RejectDuplicate       RejectCode = 0x12
	RejectNonstandard     RejectCode = 0x40
	RejectDust            RejectCode = 0x41
	RejectInsufficientFee RejectCode = 0x42
	RejectCheckpoint      RejectCode = 0x43
)

// Map of reject codes back strings for pretty printing.
var rejectCodeStrings = map[RejectCode]string{
	RejectMalformed:       "REJECT_MALFORMED",
	RejectInvalid:         "REJECT_INVALID",
	RejectObsolete:        "REJECT_OBSOLETE",
	RejectDuplicate:       "REJECT_DUPLICATE",
	RejectNonstandard:     "REJECT_NONSTANDARD",
	RejectDust:            "REJECT_DUST",
	RejectInsufficientFee: "REJECT_INSUFFICIENTFEE",
	RejectCheckpoint:      "REJECT_CHECKPOINT",
}

// String returns the RejectCode in human-readable form.
func (code RejectCode) String() string {
	if s, ok := rejectCodeStrings[code]; ok {
		return s
	}

	return fmt.Sprintf("Unknown RejectCode (%d)", uint8(code))
}

// MsgReject implements the Message interface and represents a bitcoin reject message.
type MsgReject struct {
	// Cmd is the command for the message which was rejected such as
	// as CmdBlock or CmdTx.  This can be obtained from the Command function
	// of a Message.
	Cmd string

	// RejectCode is a code indicating why the command was rejected.  It
	// is encoded as a uint8 on the wire.
	Code RejectCode

	// Reason is a human-readable string with specific details (over and
	// above the reject code) about why the command was rejected.
	Reason string
}

func (msg *MsgReject) Decode(r io.Reader, pver uint32) error {

	// Command that was rejected.
	cmd, err := wirebase.ReadVarString(r, pver)
	if err != nil {
		return err
	}
	msg.Cmd = cmd

	// Code indicating why the command was rejected.
	var code uint8
	err = wirebase.ReadElement(r, &code)
	if err != nil {
		return err
	}
	msg.Code = RejectCode(code)

	// Human readable string with specific details (over and above the
	// reject code above) about why the command was rejected.
	reason, err := wirebase.ReadVarString(r, pver)
	if err != nil {
		return err
	}
	msg.Reason = reason

	return nil
}

func (msg *MsgReject) Encode(w io.Writer, pver uint32) error {

	// Command that was rejected.
	err := wirebase.WriteVarString(w, pver, msg.Cmd)
	if err != nil {
		return err
	}

	// Code indicating why the command was rejected.
	err = wirebase.WriteElement(w, msg.Code)
	if err != nil {
		return err
	}

	// Human readable string with specific details (over and above the
	// reject code above) about why the command was rejected.
	err = wirebase.WriteVarString(w, pver, msg.Reason)
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.
// This is part of the Message interface implementation.
func (msg *MsgReject) Command() string {
	return CmdReject
}

// MaxPayloadLength returns the maximum length the payload can be for the receiver.
// This is part of the Message interface implementation.
func (msg *MsgReject) MaxPayloadLength(pver uint32) uint32 {
	plen := uint32(0)
	plen = wirebase.MaxMessagePayload

	return plen
}

// NewMsgReject returns a new reject message that conforms to the
// Message interface.  See MsgReject for details.
func NewMsgReject(command string, code RejectCode, reason string) *MsgReject {
	return &MsgReject{
		Cmd:    command,
		Code:   code,
		Reason: reason,
	}
}
