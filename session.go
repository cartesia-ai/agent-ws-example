package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	pingDeadline = 20 * time.Second
)

var (
	ErrSessionClosed = errors.New("session is closed")
)

// Session
type Session interface {
	StreamID() string
	Send(ctx context.Context, m Message) error
	Messages() <-chan Message
	Close() error
}

// session
type session struct {
	streamID string
	conn     *websocket.Conn

	cancel context.CancelFunc
	readCh chan Message
	wg     sync.WaitGroup
}

func newSession(streamID string, conn *websocket.Conn) (*session, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &session{
		streamID: streamID,
		conn:     conn,

		cancel: cancel,
		readCh: make(chan Message, 10),
	}

	s.wg.Add(2)

	go s.read(ctx)
	go s.ping(ctx)

	return s, nil
}

func (s *session) StreamID() string {
	return s.streamID
}

func (s *session) Send(ctx context.Context, m Message) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}

	log.Printf("Sending message - type: %s, len: %d", m.Type(), len(payload))

	return s.conn.Write(ctx, websocket.MessageText, payload)
}

func (s *session) Messages() <-chan Message {
	return s.readCh
}

func (s *session) Close() error {
	s.cancel()
	s.wg.Wait()

	return s.conn.Close(websocket.StatusNormalClosure, "")
}

func (s *session) read(ctx context.Context) {
	defer s.wg.Done()
	defer s.cancel()

	for {
		select {
		case <-ctx.Done():
			log.Println("Closing the read worker")
			return
		default:
		}

		_, payload, err := s.conn.Read(ctx)
		if err != nil {
			log.Printf("Error while reading message: %v", err)
			return
		}

		m, err := UnmarshalMessage(payload)
		if err != nil {
			log.Printf("Error while unmarshaling message: %v", err)
			continue
		}

		log.Printf("Received message - type: %s", m.Type())

		select {
		case s.readCh <- m:
			log.Printf("Queued message - type: %s", m.Type())
		case <-ctx.Done():
			log.Println("Closing the read worker")
			return
		}
	}
}

func (s *session) ping(ctx context.Context) {
	ticker := time.NewTicker(pingDeadline)
	defer ticker.Stop()

	defer s.wg.Done()
	defer s.cancel()

	for {
		select {
		case <-ticker.C:
			if err := s.conn.Ping(ctx); err != nil {
				log.Printf("Error while sending ping: %v", err)
			}
		case <-ctx.Done():
			log.Println("Closing the ping worker")
			return
		}
	}
}
