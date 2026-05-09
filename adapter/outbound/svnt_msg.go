package outbound

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

const (
	svntMagic            = "SVNT"
	svntAuthPlaintext    = "SVNTMSGAUTH12345"
	svntMaxMessageLength = 4 * 1024
)

type svntMsgType int

const (
	svntMsgTypeManager svntMsgType = iota
	svntMsgTypeRegisterControl
	svntMsgTypeControlAddPolicy
	svntMsgTypeControlDeletePolicy
	svntMsgTypeRegisterWork
	svntMsgTypeWorkForProxy
	svntMsgTypeTunnel
	svntMsgTypeControlKeepAlive
	svntMsgTypeControlTest
	svntMsgTypeLogLevel
)

type svntMsgStatus int

const (
	svntMsgStatusOK svntMsgStatus = iota
	svntMsgStatusFailed
	svntMsgStatusNoControl
)

type svntMsg struct {
	Magic        string        `json:"Magic"`
	Auth         string        `json:"Auth"`
	InstanceID   string        `json:"InstanceID"`
	MsgType      svntMsgType   `json:"MsgType"`
	MsgSubType   int           `json:"MsgSubType"`
	Status       svntMsgStatus `json:"Status"`
	StatusString string        `json:"StatusString"`
	PolicyID     string        `json:"PolicyID"`
	Encrypt      int           `json:"Encrypt"`
	CtrlID       int64         `json:"CtrlID"`
	TargetAddr   string        `json:"TargetAddr"`
	ClientAddr   string        `json:"ClientAddr"`
	DataString   string        `json:"DataString"`
	InstancePath []string      `json:"InstancePath"`
	IPPath       []string      `json:"IPPath"`
}

func (m *svntMsg) send(c io.ReadWriteCloser) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	var lenBuf bytes.Buffer
	if err := binary.Write(&lenBuf, binary.BigEndian, int32(len(data))); err != nil {
		return err
	}

	if err := writeFull(c, lenBuf.Bytes()); err != nil {
		return err
	}
	return writeFull(c, data)
}

func (m *svntMsg) read(c io.ReadWriteCloser) error {
	var length int32
	if err := binary.Read(c, binary.BigEndian, &length); err != nil {
		return err
	}

	if length < 0 || length > svntMaxMessageLength {
		return errors.New("invalid svnt message length")
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(c, data); err != nil {
		return err
	}

	if err := json.Unmarshal(data, m); err != nil {
		return err
	}
	if m.Magic != svntMagic {
		return errors.New("invalid svnt message")
	}
	return nil
}

func writeFull(c io.ReadWriteCloser, data []byte) error {
	for len(data) > 0 {
		n, err := c.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}
