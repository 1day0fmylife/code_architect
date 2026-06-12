package memory

import (
	"context"
	"encoding/json"
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

type ApprovalRequest struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Task      string `json:"task"`
	Status    string `json:"status"`
}

type CodeEngineRun struct {
	SessionID    string   `json:"session_id"`
	ApprovalID   string   `json:"approval_id,omitempty"`
	Agent        string   `json:"agent"`
	Engine       string   `json:"engine"`
	Command      string   `json:"cmd"`
	ReturnCode   int      `json:"returncode"`
	Status       string   `json:"status"`
	Stdout       string   `json:"stdout"`
	Stderr       string   `json:"stderr"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	DiffStat     string   `json:"diff_stat,omitempty"`
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

CREATE TABLE IF NOT EXISTS approval_requests (
    id VARCHAR(128) PRIMARY KEY,
    session_id VARCHAR(128) NOT NULL,
    agent VARCHAR(64) NOT NULL,
    task TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_approval_requests_session_id ON approval_requests(session_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_status ON approval_requests(status);

CREATE TABLE IF NOT EXISTS code_engine_runs (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(128) NOT NULL,
    approval_id VARCHAR(128),
    agent VARCHAR(64) NOT NULL,
    engine VARCHAR(32) NOT NULL,
    command TEXT NOT NULL,
    return_code INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    stdout TEXT NOT NULL,
    stderr TEXT NOT NULL,
    changed_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    diff_stat TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_code_engine_runs_session_id ON code_engine_runs(session_id);
CREATE INDEX IF NOT EXISTS idx_code_engine_runs_approval_id ON code_engine_runs(approval_id);
CREATE INDEX IF NOT EXISTS idx_code_engine_runs_status ON code_engine_runs(status);
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

func (s *Store) SaveApproval(ctx context.Context, approval ApprovalRequest) error {
	if approval.Status == "" {
		approval.Status = "pending"
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO approval_requests (id, session_id, agent, task, status)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING`,
		approval.ID, approval.SessionID, approval.Agent, approval.Task, approval.Status,
	)
	return err
}

func (s *Store) ConsumeApproval(ctx context.Context, id string) (ApprovalRequest, error) {
	var approval ApprovalRequest
	err := s.pool.QueryRow(ctx, `
UPDATE approval_requests
SET status = 'used', consumed_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING id, session_id, agent, task, status`, id).
		Scan(&approval.ID, &approval.SessionID, &approval.Agent, &approval.Task, &approval.Status)
	return approval, err
}

func (s *Store) SaveCodeEngineRun(ctx context.Context, run CodeEngineRun) error {
	changedFiles, err := json.Marshal(run.ChangedFiles)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO code_engine_runs (
    session_id, approval_id, agent, engine, command, return_code, status,
    stdout, stderr, changed_files, diff_stat
) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11)`,
		run.SessionID,
		run.ApprovalID,
		run.Agent,
		run.Engine,
		run.Command,
		run.ReturnCode,
		run.Status,
		run.Stdout,
		run.Stderr,
		string(changedFiles),
		run.DiffStat,
	)
	return err
}
