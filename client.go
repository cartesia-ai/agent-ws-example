package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// InputFormat
type InputFormat string

const (
	InputFormatMulaw8000 InputFormat = "mulaw_8000"
	InputFormatPCM16000  InputFormat = "pcm_16000"
	InputFormatPCM24000  InputFormat = "pcm_24000"
	InputFormatPCM44100  InputFormat = "pcm_44100"
)

// Config
type Config struct {
	BaseURL     string
	APIKey      string
	Version     string
	InputFormat InputFormat
}

// Client
type Client struct {
	baseURL     string
	headers     http.Header
	inputFormat InputFormat
}

func NewClient(cfg Config) (*Client, error) {
	headers := http.Header{
		"Authorization":    []string{fmt.Sprintf("Bearer %s", cfg.APIKey)},
		"Cartesia-Version": []string{cfg.Version},
	}

	return &Client{
		baseURL:     cfg.BaseURL,
		headers:     headers,
		inputFormat: cfg.InputFormat,
	}, nil
}

func (c *Client) NewSession(ctx context.Context, agentID string, metadata map[string]interface{}) (Session, error) {
	// Construct the proper URL for the agent stream endpoint
	addr := fmt.Sprintf("%s/agents/stream/%s", c.baseURL, agentID)

	opts := &websocket.DialOptions{
		HTTPHeader: c.headers,
	}

	conn, _, err := websocket.Dial(ctx, addr, opts)
	if err != nil {
		return nil, err
	}

	streamID := uuid.NewString()

	s, err := newSession(streamID, conn)
	if err != nil {
		return nil, err
	}

	start := &StartMessage{
		Event:    MessageTypeStart,
		StreamID: streamID,
		Config: StreamConfig{
			InputFormat: c.inputFormat,
		},
		Metadata: metadata,
	}

	if err := s.Send(ctx, start); err != nil {
		if closeErr := s.Close(); closeErr != nil {
			log.Printf("Failed to close session: %v", closeErr)
		}

		return nil, err
	}

	select {
	case m := <-s.Messages():
		ack, ok := m.(*AckMessage)
		if !ok {
			return nil, fmt.Errorf("expected ack message, but got %s", m.Type())
		}

		log.Printf("Handshake successful - stream_id: %s, input_format: %s",
			ack.StreamID, ack.Config.InputFormat)

		return s, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("handshake timed out: %w", ctx.Err())
	}
}
