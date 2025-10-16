# Cartesia Agent Stream WebSocket Client (Go)

Reference implementation of a Go client for Cartesia's agent stream WebSocket API. Demonstrates proper connection handling, conversation flow management, and stereo audio recording.

## Quick Start

### 1. Configure

Edit `main.go` and set your credentials:

```go
const (
    AGENT_ID = "your_agent_id_here"
    API_KEY  = "" // replace with your api key or set CARTESIA_API_KEY environment variable
)
```

### 2. Install Dependencies

```bash
go mod tidy
```

### 3. Run

```bash
go run .
```

The program will:
1. Connect to the agent and wait for initial greeting
2. Detect 2 seconds of silence after greeting
3. Send `question.wav` audio file
4. Wait for agent's response
5. Detect 2 seconds of silence after response
6. Save full conversation to `conversation_output.wav` (stereo)

## Protocol Details

### Connection Handshake

1. **Client → Server**: `start` event with configuration

```json
{
  "event": "start",
  "stream_id": "optional_custom_id",
  "config": {
    "input_format": "pcm_44100"
  }
}
```

2. **Server → Client**: `ack` event confirming handshake

```json
{
  "event": "ack",
  "stream_id": "sid_...",
  "config": {
    "input_format": "pcm_44100"
  }
}
```

### Audio Streaming

**Client → Server**: `media_input` events with base64-encoded audio

```json
{
  "event": "media_input",
  "stream_id": "sid_...",
  "media": {
    "payload": "base64_encoded_pcm_audio"
  }
}
```

**Server → Client**: `media_output` events with agent's audio response

```json
{
  "event": "media_output",
  "stream_id": "sid_...",
  "media": {
    "payload": "base64_encoded_pcm_audio"
  }
}
```

### Clear Events

The server may send `clear` events to signal buffer clearing:

```json
{
  "event": "clear",
  "stream_id": "sid_..."
}
```

**Important**: `clear` events indicate buffer management, not conversation end. Continue listening for `media_output` events.

## Audio Format

- **Format**: 16-bit PCM, mono, 44.1kHz
- **Encoding**: Base64
- **Chunk Size**: 8820 bytes (0.1 seconds at 44.1kHz × 2 bytes)
- **Streaming**: Real-time with 10ms delays between chunks

## Stereo Recording

Output WAV file uses stereo format:
- **Left channel**: User audio (your question)
- **Right channel**: Agent audio (greeting + response)

This format makes it easy to analyze the conversation timeline and verify proper turn-taking.

## Code Structure

### Creating a Client

```go
client, err := NewClient(Config{
    BaseURL:     "wss://agents.cartesia.ai",
    APIKey:      "your_api_key",
    Version:     "2025-04-16",
    InputFormat: InputFormatPCM44100,
})
```

### Creating a Session

```go
session, err := client.NewSession(ctx, agentID, nil)
defer session.Close()
```

### Sending Audio

```go
audioBytes := []byte{/* raw PCM audio */}
base64Audio := base64.StdEncoding.EncodeToString(audioBytes)

msg := &MediaInputMessage{
    Event:    MessageTypeMediaInput,
    StreamID: session.StreamID(),
    Media: Media{
        Payload: base64Audio,
    },
}

err := session.Send(ctx, msg)
```

### Receiving Responses

```go
for {
    select {
    case msg := <-session.Messages():
        switch m := msg.(type) {
        case *MediaOutputMessage:
            audioData, _ := base64.StdEncoding.DecodeString(m.Media.Payload)
            // Handle audio response
        case *ClearMessage:
            // Log and continue listening
        }
    case <-ctx.Done():
        return
    }
}
```

## Turn-Taking Implementation

The example implements natural conversation flow using silence detection:

1. **Wait for greeting**: Agent speaks first, record to right channel
2. **Detect silence**: 2 seconds of no audio signals end of greeting
3. **Send question**: Stream question audio, record to left channel
4. **Wait for response**: Agent responds, record to right channel
5. **Detect silence**: 2 seconds of no audio signals end of response
6. **Close**: Gracefully close connection

See `listenForResponses()` in `main.go:114` for implementation details.
