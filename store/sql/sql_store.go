// Package sql provides SQL-based store implementations for MySQL, PostgreSQL, and TiDB.
package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/store"
	"github.com/heibot/censor/utils"
)

// Dialect represents the SQL dialect.
type Dialect string

const (
	DialectMySQL    Dialect = "mysql"
	DialectPostgres Dialect = "postgres"
	DialectTiDB     Dialect = "tidb"
)

// Config holds the configuration for SQL store.
type Config struct {
	Dialect         Dialect
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConfig returns the default SQL store configuration.
func DefaultConfig() Config {
	return Config{
		Dialect:         DialectMySQL,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// Store implements the store.Store interface using SQL database.
type Store struct {
	db      *sql.DB
	dialect Dialect
	idGen   *utils.IDGenerator
}

// rebind converts MySQL-style placeholders (?) to the appropriate format for the dialect.
// For PostgreSQL, converts ? to $1, $2, etc.
// For MySQL/TiDB, returns the query unchanged.
func (s *Store) rebind(query string) string {
	if s.dialect != DialectPostgres {
		return query
	}

	// Convert ? to $1, $2, etc. for PostgreSQL
	var result []byte
	paramIndex := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			result = append(result, '$')
			result = append(result, []byte(fmt.Sprintf("%d", paramIndex))...)
			paramIndex++
		} else {
			result = append(result, query[i])
		}
	}
	return string(result)
}

// New creates a new SQL store.
func New(cfg Config) (*Store, error) {
	db, err := sql.Open(string(cfg.Dialect), cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{
		db:      db,
		dialect: cfg.Dialect,
		idGen:   utils.NewIDGenerator(),
	}, nil
}

// NewWithDB creates a new SQL store with an existing database connection.
func NewWithDB(db *sql.DB, dialect Dialect) *Store {
	return &Store{
		db:      db,
		dialect: dialect,
		idGen:   utils.NewIDGenerator(),
	}
}

// CreateBizReview creates a new biz review record.
func (s *Store) CreateBizReview(ctx context.Context, biz censor.BizContext) (string, error) {
	id := s.idGen.Generate()
	now := time.Now().UnixMilli()

	query := s.rebind(`INSERT INTO biz_review (id, biz_type, biz_id, field, submitter_id, trace_id, decision, status, created_at, updated_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	_, err := s.db.ExecContext(ctx, query,
		id, biz.BizType, biz.BizID, biz.Field, biz.SubmitterID, biz.TraceID,
		censor.DecisionPending, censor.StatusPending, now, now)
	if err != nil {
		return "", censor.NewStoreError("create", "biz_review", err)
	}

	return id, nil
}

// GetBizReview gets a biz review by ID.
func (s *Store) GetBizReview(ctx context.Context, bizReviewID string) (*censor.BizReview, error) {
	query := s.rebind(`SELECT id, biz_type, biz_id, field, submitter_id, trace_id, decision, status, created_at, updated_at
              FROM biz_review WHERE id = ?`)

	var br censor.BizReview
	err := s.db.QueryRowContext(ctx, query, bizReviewID).Scan(
		&br.ID, &br.BizType, &br.BizID, &br.Field, &br.SubmitterID, &br.TraceID,
		&br.Decision, &br.Status, &br.CreatedAt, &br.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, censor.ErrTaskNotFound
	}
	if err != nil {
		return nil, censor.NewStoreError("get", "biz_review", err)
	}

	return &br, nil
}

// UpdateBizDecision updates the decision for a biz review.
func (s *Store) UpdateBizDecision(ctx context.Context, bizReviewID string, decision censor.Decision) (bool, error) {
	now := time.Now().UnixMilli()

	query := s.rebind(`UPDATE biz_review SET decision = ?, updated_at = ? WHERE id = ? AND decision != ?`)
	result, err := s.db.ExecContext(ctx, query, decision, now, bizReviewID, decision)
	if err != nil {
		return false, censor.NewStoreError("update", "biz_review", err)
	}

	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

// UpdateBizStatus updates the status for a biz review.
func (s *Store) UpdateBizStatus(ctx context.Context, bizReviewID string, status censor.ReviewStatus) error {
	now := time.Now().UnixMilli()

	query := s.rebind(`UPDATE biz_review SET status = ?, updated_at = ? WHERE id = ?`)
	_, err := s.db.ExecContext(ctx, query, status, now, bizReviewID)
	if err != nil {
		return censor.NewStoreError("update", "biz_review", err)
	}

	return nil
}

// CreateResourceReview creates a new resource review record.
func (s *Store) CreateResourceReview(ctx context.Context, bizReviewID string, r censor.Resource) (string, error) {
	id := s.idGen.Generate()
	now := time.Now().UnixMilli()

	query := s.rebind(`INSERT INTO resource_review (id, biz_review_id, resource_id, resource_type, content_hash, content_text, content_url, decision, created_at, updated_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	_, err := s.db.ExecContext(ctx, query,
		id, bizReviewID, r.ResourceID, r.Type, r.ContentHash, r.ContentText, r.ContentURL,
		censor.DecisionPending, now, now)
	if err != nil {
		return "", censor.NewStoreError("create", "resource_review", err)
	}

	return id, nil
}

// GetResourceReview gets a resource review by ID.
func (s *Store) GetResourceReview(ctx context.Context, resourceReviewID string) (*censor.ResourceReview, error) {
	query := s.rebind(`SELECT id, biz_review_id, resource_id, resource_type, content_hash, content_text, content_url, decision, outcome_json, created_at, updated_at
              FROM resource_review WHERE id = ?`)

	var rr censor.ResourceReview
	var outcomeJSON sql.NullString
	err := s.db.QueryRowContext(ctx, query, resourceReviewID).Scan(
		&rr.ID, &rr.BizReviewID, &rr.ResourceID, &rr.ResourceType, &rr.ContentHash,
		&rr.ContentText, &rr.ContentURL, &rr.Decision, &outcomeJSON, &rr.CreatedAt, &rr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, censor.ErrTaskNotFound
	}
	if err != nil {
		return nil, censor.NewStoreError("get", "resource_review", err)
	}
	rr.OutcomeJSON = outcomeJSON.String

	return &rr, nil
}

// UpdateResourceOutcome updates the outcome for a resource review.
func (s *Store) UpdateResourceOutcome(ctx context.Context, resourceReviewID string, outcome censor.FinalOutcome) error {
	now := time.Now().UnixMilli()

	outcomeJSON, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("failed to marshal outcome: %w", err)
	}

	query := s.rebind(`UPDATE resource_review SET decision = ?, outcome_json = ?, updated_at = ? WHERE id = ?`)
	_, err = s.db.ExecContext(ctx, query, outcome.Decision, string(outcomeJSON), now, resourceReviewID)
	if err != nil {
		return censor.NewStoreError("update", "resource_review", err)
	}

	return nil
}

// ListResourceReviewsByBizReview lists all resource reviews for a biz review.
func (s *Store) ListResourceReviewsByBizReview(ctx context.Context, bizReviewID string) ([]censor.ResourceReview, error) {
	query := s.rebind(`SELECT id, biz_review_id, resource_id, resource_type, content_hash, content_text, content_url, decision, outcome_json, created_at, updated_at
              FROM resource_review WHERE biz_review_id = ?`)

	rows, err := s.db.QueryContext(ctx, query, bizReviewID)
	if err != nil {
		return nil, censor.NewStoreError("list", "resource_review", err)
	}
	defer rows.Close()

	var reviews []censor.ResourceReview
	for rows.Next() {
		var rr censor.ResourceReview
		var outcomeJSON sql.NullString
		if err := rows.Scan(&rr.ID, &rr.BizReviewID, &rr.ResourceID, &rr.ResourceType, &rr.ContentHash,
			&rr.ContentText, &rr.ContentURL, &rr.Decision, &outcomeJSON, &rr.CreatedAt, &rr.UpdatedAt); err != nil {
			return nil, censor.NewStoreError("scan", "resource_review", err)
		}
		rr.OutcomeJSON = outcomeJSON.String
		reviews = append(reviews, rr)
	}

	return reviews, nil
}

// CreateProviderTask creates a new provider task record.
func (s *Store) CreateProviderTask(ctx context.Context, resourceReviewID, provider, mode, remoteTaskID string, raw map[string]any) (string, error) {
	id := s.idGen.Generate()
	now := time.Now().UnixMilli()

	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("failed to marshal raw: %w", err)
	}

	query := s.rebind(`INSERT INTO provider_task (id, resource_review_id, provider, mode, remote_task_id, done, raw_json, created_at, updated_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	_, err = s.db.ExecContext(ctx, query, id, resourceReviewID, provider, mode, remoteTaskID, false, string(rawJSON), now, now)
	if err != nil {
		return "", censor.NewStoreError("create", "provider_task", err)
	}

	return id, nil
}

// GetProviderTask gets a provider task by ID.
func (s *Store) GetProviderTask(ctx context.Context, taskID string) (*censor.ProviderTask, error) {
	query := s.rebind(`SELECT id, resource_review_id, provider, mode, remote_task_id, done, result_json, raw_json, created_at, updated_at
              FROM provider_task WHERE id = ?`)

	var pt censor.ProviderTask
	var resultJSON, rawJSON sql.NullString
	err := s.db.QueryRowContext(ctx, query, taskID).Scan(
		&pt.ID, &pt.ResourceReviewID, &pt.Provider, &pt.Mode, &pt.RemoteTaskID,
		&pt.Done, &resultJSON, &rawJSON, &pt.CreatedAt, &pt.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, censor.ErrTaskNotFound
	}
	if err != nil {
		return nil, censor.NewStoreError("get", "provider_task", err)
	}
	pt.ResultJSON = resultJSON.String
	pt.RawJSON = rawJSON.String

	return &pt, nil
}

// GetProviderTaskByRemoteID gets a provider task by remote ID.
func (s *Store) GetProviderTaskByRemoteID(ctx context.Context, provider, remoteTaskID string) (*censor.ProviderTask, error) {
	query := s.rebind(`SELECT id, resource_review_id, provider, mode, remote_task_id, done, result_json, raw_json, created_at, updated_at
              FROM provider_task WHERE provider = ? AND remote_task_id = ?`)

	var pt censor.ProviderTask
	var resultJSON, rawJSON sql.NullString
	err := s.db.QueryRowContext(ctx, query, provider, remoteTaskID).Scan(
		&pt.ID, &pt.ResourceReviewID, &pt.Provider, &pt.Mode, &pt.RemoteTaskID,
		&pt.Done, &resultJSON, &rawJSON, &pt.CreatedAt, &pt.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, censor.ErrTaskNotFound
	}
	if err != nil {
		return nil, censor.NewStoreError("get", "provider_task", err)
	}
	pt.ResultJSON = resultJSON.String
	pt.RawJSON = rawJSON.String

	return &pt, nil
}

// UpdateProviderTaskResult updates the result for a provider task.
func (s *Store) UpdateProviderTaskResult(ctx context.Context, taskID string, done bool, result *censor.ReviewResult, raw map[string]any) error {
	now := time.Now().UnixMilli()

	var resultJSON, rawJSON []byte
	var err error

	if result != nil {
		resultJSON, err = json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
	}

	if raw != nil {
		rawJSON, err = json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("failed to marshal raw: %w", err)
		}
	}

	query := s.rebind(`UPDATE provider_task SET done = ?, result_json = ?, raw_json = ?, updated_at = ? WHERE id = ?`)
	_, err = s.db.ExecContext(ctx, query, done, string(resultJSON), string(rawJSON), now, taskID)
	if err != nil {
		return censor.NewStoreError("update", "provider_task", err)
	}

	return nil
}

// ListPendingAsyncTasks lists pending async tasks for a provider.
func (s *Store) ListPendingAsyncTasks(ctx context.Context, provider string, limit int) ([]censor.PendingTask, error) {
	query := s.rebind(`SELECT id, provider, remote_task_id FROM provider_task
              WHERE provider = ? AND done = 0 AND mode = 'async'
              ORDER BY created_at ASC LIMIT ?`)

	rows, err := s.db.QueryContext(ctx, query, provider, limit)
	if err != nil {
		return nil, censor.NewStoreError("list", "provider_task", err)
	}
	defer rows.Close()

	var tasks []censor.PendingTask
	for rows.Next() {
		var pt censor.PendingTask
		if err := rows.Scan(&pt.ProviderTaskID, &pt.Provider, &pt.RemoteTaskID); err != nil {
			return nil, censor.NewStoreError("scan", "provider_task", err)
		}
		tasks = append(tasks, pt)
	}

	return tasks, nil
}

// GetBinding gets the current binding for a business field.
func (s *Store) GetBinding(ctx context.Context, bizType, bizID, field string) (*censor.CensorBinding, error) {
	query := s.rebind(`SELECT id, biz_type, biz_id, field, resource_id, resource_type, content_hash, review_id,
              decision, replace_policy, replace_value, violation_ref_id, review_revision, updated_at
              FROM censor_binding WHERE biz_type = ? AND biz_id = ? AND field = ?`)

	var b censor.CensorBinding
	err := s.db.QueryRowContext(ctx, query, bizType, bizID, field).Scan(
		&b.ID, &b.BizType, &b.BizID, &b.Field, &b.ResourceID, &b.ResourceType, &b.ContentHash,
		&b.ReviewID, &b.Decision, &b.ReplacePolicy, &b.ReplaceValue, &b.ViolationRefID,
		&b.ReviewRevision, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, censor.NewStoreError("get", "censor_binding", err)
	}

	return &b, nil
}

// UpsertBinding creates or updates a binding.
func (s *Store) UpsertBinding(ctx context.Context, binding censor.CensorBinding) error {
	now := time.Now().UnixMilli()
	binding.UpdatedAt = now

	if binding.ID == "" {
		binding.ID = s.idGen.Generate()
	}

	query := s.getUpsertBindingQuery()
	_, err := s.db.ExecContext(ctx, query,
		binding.ID, binding.BizType, binding.BizID, binding.Field, binding.ResourceID, binding.ResourceType,
		binding.ContentHash, binding.ReviewID, binding.Decision, binding.ReplacePolicy, binding.ReplaceValue,
		binding.ViolationRefID, binding.ReviewRevision, now)
	if err != nil {
		return censor.NewStoreError("upsert", "censor_binding", err)
	}

	return nil
}

func (s *Store) getUpsertBindingQuery() string {
	switch s.dialect {
	case DialectPostgres:
		return `INSERT INTO censor_binding (id, biz_type, biz_id, field, resource_id, resource_type, content_hash,
                review_id, decision, replace_policy, replace_value, violation_ref_id, review_revision, updated_at)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
                ON CONFLICT (biz_type, biz_id, field) DO UPDATE SET
                resource_id = $5, resource_type = $6, content_hash = $7, review_id = $8,
                decision = $9, replace_policy = $10, replace_value = $11, violation_ref_id = $12,
                review_revision = $13, updated_at = $14`
	default: // MySQL, TiDB
		return `INSERT INTO censor_binding (id, biz_type, biz_id, field, resource_id, resource_type, content_hash,
                review_id, decision, replace_policy, replace_value, violation_ref_id, review_revision, updated_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON DUPLICATE KEY UPDATE
                resource_id = VALUES(resource_id), resource_type = VALUES(resource_type), content_hash = VALUES(content_hash),
                review_id = VALUES(review_id), decision = VALUES(decision), replace_policy = VALUES(replace_policy),
                replace_value = VALUES(replace_value), violation_ref_id = VALUES(violation_ref_id),
                review_revision = VALUES(review_revision), updated_at = VALUES(updated_at)`
	}
}

// ListBindingsByBiz lists all bindings for a business object.
func (s *Store) ListBindingsByBiz(ctx context.Context, bizType, bizID string) ([]censor.CensorBinding, error) {
	query := s.rebind(`SELECT id, biz_type, biz_id, field, resource_id, resource_type, content_hash, review_id,
              decision, replace_policy, replace_value, violation_ref_id, review_revision, updated_at
              FROM censor_binding WHERE biz_type = ? AND biz_id = ?`)

	rows, err := s.db.QueryContext(ctx, query, bizType, bizID)
	if err != nil {
		return nil, censor.NewStoreError("list", "censor_binding", err)
	}
	defer rows.Close()

	var bindings []censor.CensorBinding
	for rows.Next() {
		var b censor.CensorBinding
		if err := rows.Scan(&b.ID, &b.BizType, &b.BizID, &b.Field, &b.ResourceID, &b.ResourceType, &b.ContentHash,
			&b.ReviewID, &b.Decision, &b.ReplacePolicy, &b.ReplaceValue, &b.ViolationRefID,
			&b.ReviewRevision, &b.UpdatedAt); err != nil {
			return nil, censor.NewStoreError("scan", "censor_binding", err)
		}
		bindings = append(bindings, b)
	}

	return bindings, nil
}

// CreateBindingHistory creates a new binding history record.
func (s *Store) CreateBindingHistory(ctx context.Context, history censor.CensorBindingHistory) error {
	now := time.Now().UnixMilli()

	if history.ID == "" {
		history.ID = s.idGen.Generate()
	}

	query := s.rebind(`INSERT INTO censor_binding_history (id, biz_type, biz_id, field, resource_id, resource_type,
              decision, replace_policy, replace_value, violation_ref_id, review_revision, reason_json, source, created_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	_, err := s.db.ExecContext(ctx, query,
		history.ID, history.BizType, history.BizID, history.Field, history.ResourceID, history.ResourceType,
		history.Decision, history.ReplacePolicy, history.ReplaceValue, history.ViolationRefID,
		history.ReviewRevision, history.ReasonJSON, history.Source, now)
	if err != nil {
		return censor.NewStoreError("create", "censor_binding_history", err)
	}

	return nil
}

// ListBindingHistory lists binding history for a business field.
func (s *Store) ListBindingHistory(ctx context.Context, bizType, bizID, field string, limit int) ([]censor.CensorBindingHistory, error) {
	query := s.rebind(`SELECT id, biz_type, biz_id, field, resource_id, resource_type, decision, replace_policy,
              replace_value, violation_ref_id, review_revision, reason_json, source, created_at
              FROM censor_binding_history WHERE biz_type = ? AND biz_id = ? AND field = ?
              ORDER BY review_revision DESC LIMIT ?`)

	rows, err := s.db.QueryContext(ctx, query, bizType, bizID, field, limit)
	if err != nil {
		return nil, censor.NewStoreError("list", "censor_binding_history", err)
	}
	defer rows.Close()

	var histories []censor.CensorBindingHistory
	for rows.Next() {
		var h censor.CensorBindingHistory
		if err := rows.Scan(&h.ID, &h.BizType, &h.BizID, &h.Field, &h.ResourceID, &h.ResourceType,
			&h.Decision, &h.ReplacePolicy, &h.ReplaceValue, &h.ViolationRefID,
			&h.ReviewRevision, &h.ReasonJSON, &h.Source, &h.CreatedAt); err != nil {
			return nil, censor.NewStoreError("scan", "censor_binding_history", err)
		}
		histories = append(histories, h)
	}

	return histories, nil
}

// SaveViolationSnapshot saves a violation snapshot.
func (s *Store) SaveViolationSnapshot(ctx context.Context, biz censor.BizContext, r censor.Resource, outcome censor.FinalOutcome) (string, error) {
	id := s.idGen.Generate()
	now := time.Now().UnixMilli()

	outcomeJSON, err := json.Marshal(outcome)
	if err != nil {
		return "", fmt.Errorf("failed to marshal outcome: %w", err)
	}

	query := s.rebind(`INSERT INTO violation_snapshot (id, biz_type, biz_id, field, resource_id, resource_type,
              content_hash, content_text, content_url, outcome_json, created_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	_, err = s.db.ExecContext(ctx, query, id, biz.BizType, biz.BizID, biz.Field, r.ResourceID, r.Type,
		r.ContentHash, r.ContentText, r.ContentURL, string(outcomeJSON), now)
	if err != nil {
		return "", censor.NewStoreError("create", "violation_snapshot", err)
	}

	return id, nil
}

// GetViolationSnapshot gets a violation snapshot by ID.
func (s *Store) GetViolationSnapshot(ctx context.Context, snapshotID string) (*censor.ViolationSnapshot, error) {
	query := s.rebind(`SELECT id, biz_type, biz_id, field, resource_id, resource_type, content_hash,
              content_text, content_url, outcome_json, created_at
              FROM violation_snapshot WHERE id = ?`)

	var vs censor.ViolationSnapshot
	err := s.db.QueryRowContext(ctx, query, snapshotID).Scan(
		&vs.ID, &vs.BizType, &vs.BizID, &vs.Field, &vs.ResourceID, &vs.ResourceType,
		&vs.ContentHash, &vs.ContentText, &vs.ContentURL, &vs.OutcomeJSON, &vs.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, censor.ErrTaskNotFound
	}
	if err != nil {
		return nil, censor.NewStoreError("get", "violation_snapshot", err)
	}

	return &vs, nil
}

// ListViolationsByBiz lists violations for a business object.
func (s *Store) ListViolationsByBiz(ctx context.Context, bizType, bizID string, limit int) ([]censor.ViolationSnapshot, error) {
	query := s.rebind(`SELECT id, biz_type, biz_id, field, resource_id, resource_type, content_hash,
              content_text, content_url, outcome_json, created_at
              FROM violation_snapshot WHERE biz_type = ? AND biz_id = ?
              ORDER BY created_at DESC LIMIT ?`)

	rows, err := s.db.QueryContext(ctx, query, bizType, bizID, limit)
	if err != nil {
		return nil, censor.NewStoreError("list", "violation_snapshot", err)
	}
	defer rows.Close()

	var snapshots []censor.ViolationSnapshot
	for rows.Next() {
		var vs censor.ViolationSnapshot
		if err := rows.Scan(&vs.ID, &vs.BizType, &vs.BizID, &vs.Field, &vs.ResourceID, &vs.ResourceType,
			&vs.ContentHash, &vs.ContentText, &vs.ContentURL, &vs.OutcomeJSON, &vs.CreatedAt); err != nil {
			return nil, censor.NewStoreError("scan", "violation_snapshot", err)
		}
		snapshots = append(snapshots, vs)
	}

	return snapshots, nil
}

// Now returns the current time.
func (s *Store) Now() time.Time {
	return time.Now()
}

// WithTx executes a function within a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(store.Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	txStore := &txStore{
		Store: s,
		tx:    tx,
	}

	if err := fn(txStore); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// txStore wraps Store for transaction support.
type txStore struct {
	*Store
	tx *sql.Tx
}

// Note: In a full implementation, txStore would override all methods to use s.tx instead of s.db
