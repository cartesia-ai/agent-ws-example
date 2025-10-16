package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// Configuration
const (
	AGENT_ID   = "" // replace with your agent id
	API_KEY    = "" // replace with your api key or set CARTESIA_API_KEY environment variable
	BASE_URL   = "wss://agents.cartesia.ai"
	VERSION    = "2025-04-16"
	INPUT_WAV  = "question.wav"
	OUTPUT_WAV = "conversation_output.wav"
	CHUNK_SIZE = 8820 // 0.1 seconds at 44.1kHz * 2 bytes
)

func main() {
	apiKey := API_KEY
	if apiKey == "" {
		apiKey = os.Getenv("CARTESIA_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("Please set CARTESIA_API_KEY environment variable or configure API_KEY constant")
	}

	log.Println("🚀 Starting Cartesia agent stream test...")
	log.Printf("Input: %s | Output: %s", INPUT_WAV, OUTPUT_WAV)

	if err := runConversation(apiKey); err != nil {
		log.Fatalf("🚨 Error: %v", err)
	}

	log.Println("✅ Conversation completed successfully!")
}

// runConversation orchestrates the full conversation with audio recording
func runConversation(apiKey string) error {
	// Create client
	client, err := NewClient(Config{
		BaseURL:     BASE_URL,
		APIKey:      apiKey,
		Version:     VERSION,
		InputFormat: InputFormatPCM44100,
	})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Create session
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session, err := client.NewSession(ctx, AGENT_ID, nil)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Initialize stereo audio recorder (left=user, right=agent)
	recorder, err := NewDualChannelRecorder(OUTPUT_WAV, 44100)
	if err != nil {
		return fmt.Errorf("failed to create recorder: %w", err)
	}
	defer recorder.Close()

	// Coordination channels
	sendQuestion := make(chan struct{})     // Signals when to send question
	questionComplete := make(chan struct{}) // Signals question was sent
	responseDone := make(chan error, 1)     // Signals conversation complete

	// Start listener goroutine
	go func() {
		responseDone <- listenForResponses(ctx, session, recorder, sendQuestion, questionComplete)
	}()

	// Wait for agent's initial greeting to complete
	select {
	case <-sendQuestion:
		log.Println("📤 Sending question...")
	case err := <-responseDone:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	// Send question audio
	if err := sendAudioFile(ctx, session, INPUT_WAV, recorder); err != nil {
		return fmt.Errorf("failed to send audio: %w", err)
	}
	close(questionComplete)

	// Wait for conversation to complete
	if err := <-responseDone; err != nil {
		return err
	}

	log.Printf("💾 Audio saved: %s", OUTPUT_WAV)
	return nil
}

// listenForResponses handles the conversation flow by monitoring agent audio
// and coordinating turn-taking between agent greeting, user question, and agent response.
func listenForResponses(ctx context.Context, session Session, recorder *DualChannelRecorder, sendQuestion, questionComplete chan struct{}) error {
	var (
		greetingComplete = false
		questionSent     = false
		agentSpeaking    = false
		lastAudioTime    = time.Now()
		silenceThreshold = 2 * time.Second
		responseTimeout  = 10 * time.Second
	)

	for {
		select {
		case msg, ok := <-session.Messages():
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			switch m := msg.(type) {
			case *MediaOutputMessage:
				audioData, err := base64.StdEncoding.DecodeString(m.Media.Payload)
				if err != nil {
					log.Printf("⚠️  Decode error: %v", err)
					continue
				}

				if len(audioData) > 0 {
					if err := recorder.WriteRight(audioData); err != nil {
						return fmt.Errorf("write audio error: %w", err)
					}
					agentSpeaking = true
					lastAudioTime = time.Now()
				}

			case *ClearMessage:
				// Clear indicates agent buffer was cleared, not end of conversation
				log.Println("🔚 Clear event received")
			}

		case <-questionComplete:
			if !questionSent {
				log.Println("📬 Question sent, waiting for response...")
				questionSent = true
				agentSpeaking = false
				lastAudioTime = time.Now()
			}
			questionComplete = nil // Prevent repeat triggers

		case <-time.After(100 * time.Millisecond):
			elapsed := time.Since(lastAudioTime)

			// Initial greeting complete: 2s silence after agent starts speaking
			if agentSpeaking && !greetingComplete && elapsed > silenceThreshold {
				log.Println("✅ Greeting complete")
				greetingComplete = true
				close(sendQuestion)
				agentSpeaking = false
			}

			// Response complete: 2s silence after agent responds to question
			if greetingComplete && questionSent && agentSpeaking && elapsed > silenceThreshold {
				log.Println("✅ Response complete")
				return nil
			}

			// Timeout: no response after 10s
			if greetingComplete && questionSent && !agentSpeaking && elapsed > responseTimeout {
				log.Printf("⚠️  No response after %.0fs", responseTimeout.Seconds())
				return nil
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// sendAudioFile streams an audio file to the agent in real-time chunks
// and records it to the left channel of the output.
func sendAudioFile(ctx context.Context, session Session, filename string, recorder *DualChannelRecorder) error {
	audioData, err := readWAVData(filename)
	if err != nil {
		return fmt.Errorf("read WAV error: %w", err)
	}

	// Send audio in chunks
	for offset := 0; offset < len(audioData); offset += CHUNK_SIZE {
		end := min(offset+CHUNK_SIZE, len(audioData))
		chunk := audioData[offset:end]

		// Record to left channel
		if err := recorder.WriteLeft(chunk); err != nil {
			return fmt.Errorf("write audio error: %w", err)
		}

		// Send to agent
		base64Audio := base64.StdEncoding.EncodeToString(chunk)
		mediaMsg := &MediaInputMessage{
			Event:    MessageTypeMediaInput,
			StreamID: session.StreamID(),
			Media:    Media{Payload: base64Audio},
		}

		if err := session.Send(ctx, mediaMsg); err != nil {
			return fmt.Errorf("send audio error: %w", err)
		}

		// Simulate real-time streaming (10ms per 0.1s chunk)
		time.Sleep(10 * time.Millisecond)
	}

	// Send 1 second of silence to signal end of turn
	silenceChunk := make([]byte, CHUNK_SIZE)
	for i := 0; i < 10; i++ {
		recorder.WriteLeft(silenceChunk)

		base64Silence := base64.StdEncoding.EncodeToString(silenceChunk)
		silenceMsg := &MediaInputMessage{
			Event:    MessageTypeMediaInput,
			StreamID: session.StreamID(),
			Media:    Media{Payload: base64Silence},
		}

		if err := session.Send(ctx, silenceMsg); err != nil {
			return fmt.Errorf("send silence error: %w", err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// readWAVData extracts PCM audio data from a WAV file (skips 44-byte header).
func readWAVData(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := file.Seek(44, io.SeekStart); err != nil {
		return nil, err
	}

	return io.ReadAll(file)
}

// DualChannelRecorder records stereo audio with separate left/right channels.
// Left channel: user audio, Right channel: agent audio.
type DualChannelRecorder struct {
	file       *os.File
	encoder    *wav.Encoder
	sampleRate int
}

// NewDualChannelRecorder creates a stereo WAV recorder.
func NewDualChannelRecorder(filename string, sampleRate int) (*DualChannelRecorder, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	encoder := wav.NewEncoder(file, sampleRate, 16, 2, 1)

	return &DualChannelRecorder{
		file:       file,
		encoder:    encoder,
		sampleRate: sampleRate,
	}, nil
}

// WriteLeft writes user audio to the left channel (right channel = silence).
func (r *DualChannelRecorder) WriteLeft(data []byte) error {
	return r.writeChannel(data, true)
}

// WriteRight writes agent audio to the right channel (left channel = silence).
func (r *DualChannelRecorder) WriteRight(data []byte) error {
	return r.writeChannel(data, false)
}

// writeChannel writes audio to one channel with silence on the other.
func (r *DualChannelRecorder) writeChannel(data []byte, left bool) error {
	samples := bytesToInt16(data)
	interleavedData := make([]int, len(samples)*2)

	for i := 0; i < len(samples); i++ {
		if left {
			interleavedData[i*2] = int(samples[i]) // Left
			interleavedData[i*2+1] = 0              // Right silence
		} else {
			interleavedData[i*2] = 0                // Left silence
			interleavedData[i*2+1] = int(samples[i]) // Right
		}
	}

	buf := &audio.IntBuffer{
		Data:   interleavedData,
		Format: &audio.Format{SampleRate: r.sampleRate, NumChannels: 2},
	}

	return r.encoder.Write(buf)
}

// Close finalizes and closes the WAV file.
func (r *DualChannelRecorder) Close() error {
	if err := r.encoder.Close(); err != nil {
		r.file.Close()
		return err
	}
	return r.file.Close()
}

// bytesToInt16 converts bytes to int16 samples (little-endian).
func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}
