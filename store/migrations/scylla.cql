-- Censor System Database Schema for ScyllaDB/Cassandra
-- ScyllaDB uses a different data model optimized for query patterns
-- Execute with: cqlsh -f scylla.cql

-- Create keyspace (adjust replication for production)
CREATE KEYSPACE IF NOT EXISTS censor
WITH replication = {
    'class': 'SimpleStrategy',
    'replication_factor': 1
};

USE censor;

-- ============================================================
-- Table: biz_review_by_id
-- Purpose: Lookup biz review by ID
-- ============================================================
CREATE TABLE IF NOT EXISTS biz_review_by_id (
    id              TEXT PRIMARY KEY,
    biz_type        TEXT,
    biz_id          TEXT,
    field           TEXT,
    submitter_id    TEXT,
    trace_id        TEXT,
    decision        TEXT,
    status          TEXT,
    created_at      BIGINT,
    updated_at      BIGINT
);

-- ============================================================
-- Table: biz_review_by_biz
-- Purpose: Query reviews by business object
-- ============================================================
CREATE TABLE IF NOT EXISTS biz_review_by_biz (
    biz_type        TEXT,
    biz_id          TEXT,
    created_at      BIGINT,
    id              TEXT,
    field           TEXT,
    submitter_id    TEXT,
    trace_id        TEXT,
    decision        TEXT,
    status          TEXT,
    updated_at      BIGINT,
    PRIMARY KEY ((biz_type, biz_id), created_at, id)
) WITH CLUSTERING ORDER BY (created_at DESC, id ASC);

-- ============================================================
-- Table: resource_review_by_id
-- Purpose: Lookup resource review by ID
-- ============================================================
CREATE TABLE IF NOT EXISTS resource_review_by_id (
    id              TEXT PRIMARY KEY,
    biz_review_id   TEXT,
    resource_id     TEXT,
    resource_type   TEXT,
    content_hash    TEXT,
    content_text    TEXT,
    content_url     TEXT,
    decision        TEXT,
    outcome_json    TEXT,
    created_at      BIGINT,
    updated_at      BIGINT
);

-- ============================================================
-- Table: resource_review_by_biz_review
-- Purpose: List resources for a biz review
-- ============================================================
CREATE TABLE IF NOT EXISTS resource_review_by_biz_review (
    biz_review_id   TEXT,
    resource_id     TEXT,
    id              TEXT,
    resource_type   TEXT,
    content_hash    TEXT,
    decision        TEXT,
    outcome_json    TEXT,
    created_at      BIGINT,
    updated_at      BIGINT,
    PRIMARY KEY (biz_review_id, resource_id, id)
);

-- ============================================================
-- Table: provider_task_by_id
-- Purpose: Lookup provider task by ID
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_task_by_id (
    id                  TEXT PRIMARY KEY,
    resource_review_id  TEXT,
    provider            TEXT,
    mode                TEXT,
    remote_task_id      TEXT,
    done                BOOLEAN,
    result_json         TEXT,
    raw_json            TEXT,
    created_at          BIGINT,
    updated_at          BIGINT
);

-- ============================================================
-- Table: provider_task_pending
-- Purpose: Poll pending async tasks by provider
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_task_pending (
    provider        TEXT,
    created_at      BIGINT,
    id              TEXT,
    resource_review_id TEXT,
    remote_task_id  TEXT,
    mode            TEXT,
    PRIMARY KEY (provider, created_at, id)
) WITH CLUSTERING ORDER BY (created_at ASC, id ASC);

-- ============================================================
-- Table: provider_task_by_remote
-- Purpose: Lookup by provider's task ID (for callbacks)
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_task_by_remote (
    provider        TEXT,
    remote_task_id  TEXT,
    id              TEXT,
    resource_review_id TEXT,
    PRIMARY KEY ((provider, remote_task_id))
);

-- ============================================================
-- Table: censor_binding
-- Purpose: Current moderation state
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding (
    biz_type        TEXT,
    biz_id          TEXT,
    field           TEXT,
    id              TEXT,
    resource_id     TEXT,
    resource_type   TEXT,
    content_hash    TEXT,
    review_id       TEXT,
    decision        TEXT,
    replace_policy  TEXT,
    replace_value   TEXT,
    violation_ref_id TEXT,
    review_revision INT,
    updated_at      BIGINT,
    PRIMARY KEY ((biz_type, biz_id), field)
);

-- ============================================================
-- Table: censor_binding_by_biz
-- Purpose: List all bindings for a business object
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding_by_biz (
    biz_type        TEXT,
    biz_id          TEXT,
    field           TEXT,
    id              TEXT,
    decision        TEXT,
    replace_policy  TEXT,
    replace_value   TEXT,
    review_revision INT,
    updated_at      BIGINT,
    PRIMARY KEY ((biz_type, biz_id), field)
);

-- ============================================================
-- Table: censor_binding_history
-- Purpose: Historical state changes
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding_history (
    biz_type        TEXT,
    biz_id          TEXT,
    field           TEXT,
    review_revision INT,
    id              TEXT,
    resource_id     TEXT,
    resource_type   TEXT,
    decision        TEXT,
    replace_policy  TEXT,
    replace_value   TEXT,
    violation_ref_id TEXT,
    reason_json     TEXT,
    source          TEXT,
    created_at      BIGINT,
    PRIMARY KEY ((biz_type, biz_id, field), review_revision)
) WITH CLUSTERING ORDER BY (review_revision DESC);

-- ============================================================
-- Table: violation_snapshot_by_id
-- Purpose: Lookup violation by ID
-- ============================================================
CREATE TABLE IF NOT EXISTS violation_snapshot_by_id (
    id              TEXT PRIMARY KEY,
    biz_type        TEXT,
    biz_id          TEXT,
    field           TEXT,
    resource_id     TEXT,
    resource_type   TEXT,
    content_hash    TEXT,
    content_text    TEXT,
    content_url     TEXT,
    outcome_json    TEXT,
    created_at      BIGINT
);

-- ============================================================
-- Table: violation_snapshot_by_biz
-- Purpose: List violations for a business object
-- ============================================================
CREATE TABLE IF NOT EXISTS violation_snapshot_by_biz (
    biz_type        TEXT,
    biz_id          TEXT,
    created_at      BIGINT,
    id              TEXT,
    field           TEXT,
    resource_type   TEXT,
    content_hash    TEXT,
    outcome_json    TEXT,
    PRIMARY KEY ((biz_type, biz_id), created_at, id)
) WITH CLUSTERING ORDER BY (created_at DESC, id ASC);
