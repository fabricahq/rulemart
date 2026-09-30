// Package hello is the walking skeleton's shared code: the message the web function queues, and the store where
// the worker keeps it.
package hello

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// MaxTextLength bounds a message's text, well under SQS's message size limit.
const MaxTextLength = 1000

// Message is the SQS message body the web function sends and the worker stores.
type Message struct {
	Text   string    `json:"text"`
	Source string    `json:"source"`
	SentAt time.Time `json:"sentAt"`
}

// Validate rejects a message the web function could not have sent, such as an empty or truncated body.
func (m Message) Validate() error {
	switch {
	case m.Text == "":
		return errors.New("message has no text")
	case utf8.RuneCountInString(m.Text) > MaxTextLength:
		return fmt.Errorf("message text is longer than %d characters", MaxTextLength)
	case m.Source == "":
		return errors.New("message has no source")
	case m.SentAt.IsZero():
		return errors.New("message has no sent time")
	}
	return nil
}

// Row is a stored message as the web function reports it.
type Row struct {
	ID         int64     `json:"id"`
	MessageID  string    `json:"messageId"`
	Text       string    `json:"text"`
	Source     string    `json:"source"`
	SentAt     time.Time `json:"sentAt"`
	ReceivedAt time.Time `json:"receivedAt"`
}
