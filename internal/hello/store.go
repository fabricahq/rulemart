// Store keeps received messages in Postgres.

package hello

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/database"
)

// Store keeps received messages in the hello_messages table. Both operations are safe to retry, so they recover
// from a changed database password.
type Store struct {
	db *database.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

// Insert stores m once for SQS message messageID. SQS delivers at least once, so storing the same ID again changes
// nothing.
func (s *Store) Insert(ctx context.Context, messageID string, m Message) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		_, err := pool.Exec(ctx, `INSERT INTO hello_messages (message_id, text, source, sent_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT (message_id) DO NOTHING`, messageID, m.Text, m.Source, m.SentAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("store message messageID=%q: %v", messageID, err)
	}
	return nil
}

// Latest returns up to limit stored messages, newest first.
func (s *Store) Latest(ctx context.Context, limit int) ([]Row, error) {
	var rows []Row
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		result, err := pool.Query(ctx, `SELECT id, message_id, text, source, sent_at, received_at FROM hello_messages
			ORDER BY id DESC LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer result.Close()
		rows = []Row{}
		for result.Next() {
			var r Row
			if err := result.Scan(&r.ID, &r.MessageID, &r.Text, &r.Source, &r.SentAt, &r.ReceivedAt); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return result.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list latest messages limit=%d: %v", limit, err)
	}
	return rows, nil
}
