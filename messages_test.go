package anthropic

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func TestMessages(t *testing.T) {
	client := NewClient()
	res, err := client.CreateMessage(context.Background(), MessageCreateParams{
		Model:     ModelClaude3Sonnet,
		MaxTokens: 4000,
		System:    "You are in test mode. You're job is to reply Ok to the user message. Only return 'Ok'",
		Messages: []MessageParam{
			{Role: RoleUser, Content: "Reply to this with only 'Ok'"},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, res.Content[0].Text, "Ok")
}

func TestMessagesStream(t *testing.T) {
	client := NewClient()
	res, err := client.StreamMessage(context.Background(), MessageCreateParams{
		Model:     ModelClaude3Sonnet,
		MaxTokens: 4000,
		System:    "You are in test mode. You're job is to reply Ok to the user message. Only return 'Ok'",
		Messages: []MessageParam{
			{Role: RoleUser, Content: "Reply to this with only 'Ok'"},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)

	content := ""
	for {
		m, err := res.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		assert.NoError(t, err)
		assert.NotNil(t, m)

		if m.Delta != nil && m.Delta.StopReason != "" {
			break
		}

		if m.ContentBlock != nil {
			content += m.ContentBlock.Text
		}
	}

	assert.Equal(t, "Ok", content)
}
