// Package hello is the walking skeleton's shared code: the message the web function queues, and the store where
// the worker keeps it.
package hello

import (
	"errors"
	"fmt"
	"strings"
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

// Validate rejects a message the worker couldn't store, such as an empty or truncated body, or text Postgres can't
// hold. The web function checks each message before queueing it, and the worker again after receiving it. Its errors
// describe only the message, so they're safe to show the sender.
func (m Message) Validate() error {
	switch {
	case m.Text == "":
		return errors.New("message has no text")
	case utf8.RuneCountInString(m.Text) > MaxTextLength:
		return fmt.Errorf("message text is longer than %d characters", MaxTextLength)
	case !storable(m.Text):
		return errors.New("message text must be valid UTF-8 without NUL characters")
	case m.Source == "":
		return errors.New("message has no source")
	case !storable(m.Source):
		return errors.New("message source must be valid UTF-8 without NUL characters")
	case m.SentAt.IsZero():
		return errors.New("message has no sent time")
	}
	return nil
}

// storable reports whether a Postgres text column can hold s: it must be valid UTF-8 and contain no NUL character.
func storable(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
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
