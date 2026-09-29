// Package hello is the walking skeleton's shared code: the message the web function queues and the worker stores,
// and the Neon connection both functions open from an SSM parameter.
package hello

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Message is the SQS message body the web function sends and the worker stores.
type Message struct {
	Text   string    `json:"text"`
	Source string    `json:"source"`
	SentAt time.Time `json:"sentAt"`
}

// Row is a stored message as the web function reports it.
type Row struct {
	ID         int64     `json:"id"`
	Text       string    `json:"text"`
	Source     string    `json:"source"`
	SentAt     time.Time `json:"sentAt"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// OpenDB reads the Neon connection string from the SecureString parameter named by DATABASE_URL_PARAMETER,
// connects, and creates the messages table if it's missing. The connection string never appears in Terraform
// state or in the function's configuration.
func OpenDB(ctx context.Context) (*pgxpool.Pool, error) {
	name := os.Getenv("DATABASE_URL_PARAMETER")
	if name == "" {
		return nil, errors.New("DATABASE_URL_PARAMETER is not set")
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	out, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name), WithDecryption: aws.Bool(true)})
	if err != nil {
		return nil, fmt.Errorf("read parameter %s: %w", name, err)
	}
	url := aws.ToString(out.Parameter.Value)
	if !strings.HasPrefix(url, "postgres") {
		return nil, fmt.Errorf("parameter %s doesn't hold a Postgres connection string yet", name)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to Neon: %w", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS hello_messages (
		id          bigserial PRIMARY KEY,
		text        text NOT NULL,
		source      text NOT NULL,
		sent_at     timestamptz NOT NULL,
		received_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create hello_messages: %w", err)
	}
	return pool, nil
}

// Insert stores one received message.
func Insert(ctx context.Context, db *pgxpool.Pool, m Message) error {
	_, err := db.Exec(ctx, `INSERT INTO hello_messages (text, source, sent_at) VALUES ($1, $2, $3)`, m.Text, m.Source, m.SentAt)
	return err
}

// Latest returns the most recently received messages, newest first.
func Latest(ctx context.Context, db *pgxpool.Pool, limit int) ([]Row, error) {
	rows, err := db.Query(ctx, `SELECT id, text, source, sent_at, received_at FROM hello_messages ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Text, &r.Source, &r.SentAt, &r.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
