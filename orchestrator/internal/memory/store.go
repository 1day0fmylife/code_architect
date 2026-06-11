package memory

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS memory_events (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(128) NOT NULL,
    role VARCHAR(64) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_events_session_id ON memory_events(session_id);
CREATE INDEX IF NOT EXISTS idx_memory_events_role ON memory_events(role);
CREATE INDEX IF NOT EXISTS idx_memory_events_created_at ON memory_events(created_at);
`)
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Remember(ctx context.Context, sessionID, role, content string) error {
	if len(content) > 20000 {
		content = content[:20000]
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO memory_events (session_id, role, content) VALUES ($1, $2, $3)`,
		sessionID, role, content,
	)
	return err
}

func (s *Store) Recall(ctx context.Context, sessionID string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, session_id, role, content, created_at
FROM (
    SELECT id, session_id, role, content, created_at
    FROM memory_events
    WHERE session_id = $1
    ORDER BY id DESC
    LIMIT $2
) AS recent
ORDER BY id ASC`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.SessionID, &event.Role, &event.Content, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
