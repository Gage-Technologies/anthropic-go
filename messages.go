package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type StreamEvent string

const (
	StreamEventPing              StreamEvent = "ping"
	StreamEventError             StreamEvent = "error"
	StreamEventMessageStart      StreamEvent = "message_start"
	StreamEventMessageStop       StreamEvent = "message_stop"
	StreamEventMessageDelta      StreamEvent = "message_delta"
	StreamEventContentBlockStart StreamEvent = "content_block_start"
	StreamEventContentBlockStop  StreamEvent = "content_block_stop"
	StreamEventContentBlockDelta StreamEvent = "content_block_delta"
)

type MessageStreamEvent struct {
	Type         StreamEvent   `json:"type"`
	Message      *Message      `json:"message,omitempty"`
	Delta        *MessageDelta `json:"delta,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Index        int           `json:"index,omitempty"`
}

type Message struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type MessageDeltaWrapper struct {
	Type  string       `json:"type"`
	Delta MessageDelta `json:"delta"`
	Usage *Usage       `json:"usage,omitempty"`
}

type MessageDelta struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

type ContentBlockDelta struct {
	Type  string    `json:"type"`
	Index int       `json:"index"`
	Delta TextDelta `json:"delta"`
}

type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MessageCreateParams struct {
	MaxTokens     int               `json:"max_tokens"`
	Messages      []MessageParam    `json:"messages"`
	Model         string            `json:"model"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	System        string            `json:"system,omitempty"`
	Temperature   float64           `json:"temperature,omitempty"`
	TopK          int               `json:"top_k,omitempty"`
	TopP          float64           `json:"top_p,omitempty"`
}

type MessageParam struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Client) CreateMessage(ctx context.Context, params MessageCreateParams) (*Message, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/v1/messages", params)
	if err != nil {
		return nil, err
	}

	var msg Message
	_, err = c.do(req, &msg)
	if err != nil {
		return nil, err
	}

	return &msg, nil
}

func (c *Client) StreamMessage(ctx context.Context, params MessageCreateParams) (*MessageStream, error) {
	params.Stream = true

	req, err := c.newRequest(ctx, http.MethodPost, "/v1/messages", params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", c.streamAccept)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: %s", resp.Status)
	}

	return &MessageStream{
		resp:                resp,
		reader:              bufio.NewReader(resp.Body),
		ignoreUnknownEvents: true,
	}, nil
}

type MessageStream struct {
	resp                *http.Response
	reader              *bufio.Reader
	event               MessageStreamEvent
	ignoreUnknownEvents bool
}

func (s *MessageStream) Close() error {
	return s.resp.Body.Close()
}

func (s *MessageStream) ErrorUnknownEvent() {
	s.ignoreUnknownEvents = false
}

func (s *MessageStream) Recv() (*MessageStreamEvent, error) {
	var eventType StreamEvent
	var data strings.Builder

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			// skip pings since the caller doesn't care
			if eventType == StreamEventPing {
				data.Reset()
				continue
			}
			break
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SSE format: %s", line)
		}

		field, value := parts[0], parts[1]
		switch field {
		case "event":
			eventType = StreamEvent(value)
		case "data":
			data.WriteString(value)
			data.WriteString("\n")
		default:
			// Ignore unknown fields
		}
	}

	if data.Len() > 0 {
		s.event.Type = eventType
		switch eventType {
		case StreamEventMessageStart, StreamEventMessageStop:
			if err := json.Unmarshal([]byte(data.String()), &s.event); err != nil {
				return nil, err
			}
		case StreamEventMessageDelta:
			var delta MessageDeltaWrapper
			if err := json.Unmarshal([]byte(data.String()), &delta); err != nil {
				return nil, err
			}
			s.event.Delta = &delta.Delta
			if s.event.Message != nil {
				s.event.Message.Usage.OutputTokens += delta.Usage.OutputTokens
			}
		case StreamEventContentBlockStart, StreamEventContentBlockStop:
			var contentBlock ContentBlock
			if err := json.Unmarshal([]byte(data.String()), &contentBlock); err != nil {
				return nil, err
			}
			s.event.ContentBlock = &contentBlock
		case StreamEventContentBlockDelta:
			var delta ContentBlockDelta
			if err := json.Unmarshal([]byte(data.String()), &delta); err != nil {
				return nil, err
			}
			s.event.ContentBlock = &ContentBlock{
				Type: delta.Delta.Type,
				Text: delta.Delta.Text,
			}
			s.event.Index = delta.Index
		case StreamEventError:
			return nil, fmt.Errorf("stream error: %s", data.String())
		default:
			if !s.ignoreUnknownEvents {
				return nil, fmt.Errorf("unknown event type: %s", eventType)
			}
		}
	}

	return &s.event, nil
}
