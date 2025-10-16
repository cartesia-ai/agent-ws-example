package main

import (
	"encoding/json"
	"errors"
)

var (
	ErrUnknownMessageType = errors.New("unknown message type")
)

// MessageType
type MessageType string

const (
	MessageTypeStart       MessageType = "start"
	MessageTypeAck         MessageType = "ack"
	MessageTypeMediaInput  MessageType = "media_input"
	MessageTypeDTMF        MessageType = "dtmf"
	MessageTypeCustom      MessageType = "custom"
	MessageTypeMediaOutput MessageType = "media_output"
	MessageTypeClear       MessageType = "clear"
)

// Message
type Message interface {
	Type() MessageType
}

// StartMessage
type StartMessage struct {
	Event    MessageType  `json:"event"`
	StreamID string       `json:"stream_id"`
	Config   StreamConfig `json:"config"`
	Metadata Metadata     `json:"metadata"`
}

func (m *StartMessage) Type() MessageType {
	return MessageTypeStart
}

// AckMessage
type AckMessage struct {
	Event    MessageType  `json:"event"`
	StreamID string       `json:"stream_id"`
	Config   StreamConfig `json:"config"`
}

func (m *AckMessage) Type() MessageType {
	return MessageTypeAck
}

// MediaInputMessage
type MediaInputMessage struct {
	Event    MessageType `json:"event"`
	StreamID string      `json:"stream_id"`
	Media    Media       `json:"media"`
}

func (m *MediaInputMessage) Type() MessageType {
	return MessageTypeMediaInput
}

// DTMFMessage
type DTMFMessage struct {
	Event    MessageType `json:"event"`
	StreamID string      `json:"stream_id"`
	DTMF     string      `json:"dtmf"`
}

func (m *DTMFMessage) Type() MessageType {
	return MessageTypeDTMF
}

// CustomMessage
type CustomMessage struct {
	Event    MessageType `json:"event"`
	StreamID string      `json:"stream_id"`
	Metadata Metadata    `json:"metadata"`
}

func (m *CustomMessage) Type() MessageType {
	return MessageTypeCustom
}

// MediaOutputMessage
type MediaOutputMessage struct {
	Event    MessageType `json:"event"`
	StreamID string      `json:"stream_id"`
	Media    Media       `json:"media"`
}

func (m *MediaOutputMessage) Type() MessageType {
	return MessageTypeMediaOutput
}

// ClearMessage
type ClearMessage struct {
	Event    MessageType `json:"event"`
	StreamID string      `json:"stream_id"`
}

func (m *ClearMessage) Type() MessageType {
	return MessageTypeClear
}

// Metadata
type Metadata map[string]interface{}

// Media
type Media struct {
	Payload string `json:"payload"`
}

// StreamConfig
type StreamConfig struct {
	InputFormat InputFormat `json:"input_format"`
}

// UnmarshalMessage
func UnmarshalMessage(data []byte) (Message, error) {
	var genericMsg struct {
		Event MessageType `json:"event"`
	}

	if err := json.Unmarshal(data, &genericMsg); err != nil {
		return nil, err
	}

	var msg Message
	switch genericMsg.Event {
	case MessageTypeAck:
		msg = &AckMessage{}
	case MessageTypeMediaOutput:
		msg = &MediaOutputMessage{}
	case MessageTypeClear:
		msg = &ClearMessage{}
	case MessageTypeDTMF:
		msg = &DTMFMessage{}
	case MessageTypeCustom:
		msg = &CustomMessage{}
	}

	if msg == nil {
		return nil, ErrUnknownMessageType
	}

	if err := json.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	return msg, nil
}
