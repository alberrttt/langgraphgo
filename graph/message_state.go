package graph

import (
	"github.com/tmc/langchaingo/llms"
)

type MessageState struct {
	Messages []llms.MessageContent
}

func NewMessageState() MessageState {
	return MessageState{
		Messages: []llms.MessageContent{},
	}
}
func (s *MessageState) AddMessage(message llms.MessageContent) {
	s.Messages = append(s.Messages, message)
}

func (s *MessageState) LastMessage() llms.MessageContent {
	return s.Messages[len(s.Messages)-1]
}

func (s *MessageState) LastMessageOfRole(role llms.ChatMessageType) llms.MessageContent {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == role {
			return s.Messages[i]
		}
	}
	panic("no message of role " + role)
}
