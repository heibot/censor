-- Censor System Database Schema for PostgreSQL
-- Execute this file to initialize the database tables.

-- ============================================================
-- Table: biz_review
-- Purpose: Stores business-level review records
-- ============================================================
CREATE TABLE IF NOT EXISTS biz_review (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    submitter_id    VARCHAR(128) NOT NULL,
    trace_id        VARCHAR(128) NOT NULL,
    decision        VARCHAR(16) NOT NULL DEFAULT 'pending',
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_biz_review_biz ON biz_review (biz_type, biz_id);
CREATE INDEX IF NOT EXISTS idx_biz_review_status ON biz_review (status, decision);
CREATE INDEX IF NOT EXISTS idx_biz_review_created ON biz_review (created_at);
CREATE INDEX IF NOT EXISTS idx_biz_review_submitter ON biz_review (submitter_id, created_at);

COMMENT ON TABLE biz_review IS 'Business-level review records';
COMMENT ON COLUMN biz_review.biz_type IS 'Business type: user_avatar, note_body, etc.';
COMMENT ON COLUMN biz_review.decision IS 'pass/review/block/error/pending';
COMMENT ON COLUMN biz_review.status IS 'pending/running/done/failed/canceled';

-- ============================================================
-- Table: resource_review
-- Purpose: Stores resource-level review records
-- ============================================================
CREATE TABLE IF NOT EXISTS resource_review (
    id              VARCHAR(64) PRIMARY KEY,
    biz_review_id   VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    content_text    TEXT NULL,
    content_url     TEXT NULL,
    decision        VARCHAR(16) NOT NULL DEFAULT 'pending',
    outcome_json    JSONB NULL,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_resource_review_biz ON resource_review (biz_review_id);
CREATE INDEX IF NOT EXISTS idx_resource_review_hash ON resource_review (content_hash);
CREATE INDEX IF NOT EXISTS idx_resource_review_decision ON resource_review (decision);
CREATE INDEX IF NOT EXISTS idx_resource_review_resource ON resource_review (resource_id);

COMMENT ON TABLE resource_review IS 'Resource-level review records';
COMMENT ON COLUMN resource_review.resource_type IS 'text/image/video';

-- ============================================================
-- Table: provider_task
-- Purpose: Stores provider task records
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_task (
    id                  VARCHAR(64) PRIMARY KEY,
    resource_review_id  VARCHAR(64) NOT NULL,
    provider            VARCHAR(32) NOT NULL,
    mode                VARCHAR(8) NOT NULL,
    remote_task_id      VARCHAR(128) NOT NULL,
    done                BOOLEAN NOT NULL DEFAULT FALSE,
    result_json         JSONB NULL,
    raw_json            JSONB NULL,
    created_at          BIGINT NOT NULL,
    updated_at          BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_provider_task_resource ON provider_task (resource_review_id);
CREATE INDEX IF NOT EXISTS idx_provider_task_provider_done ON provider_task (provider, done);
CREATE INDEX IF NOT EXISTS idx_provider_task_remote ON provider_task (provider, remote_task_id);
CREATE INDEX IF NOT EXISTS idx_provider_task_pending ON provider_task (done, mode, created_at);

COMMENT ON TABLE provider_task IS 'Provider API call records';
COMMENT ON COLUMN provider_task.provider IS 'aliyun/huawei/tencent/manual';
COMMENT ON COLUMN provider_task.mode IS 'sync/async';

-- ============================================================
-- Table: censor_binding
-- Purpose: Current moderation state for business fields
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    review_id       VARCHAR(128) NOT NULL,
    decision        VARCHAR(16) NOT NULL,
    replace_policy  VARCHAR(32) NULL,
    replace_value   VARCHAR(255) NULL,
    violation_ref_id VARCHAR(128) NULL,
    review_revision INT NOT NULL DEFAULT 1,
    updated_at      BIGINT NOT NULL,

    CONSTRAINT uq_censor_binding_biz_field UNIQUE (biz_type, biz_id, field)
);

CREATE INDEX IF NOT EXISTS idx_censor_binding_biz ON censor_binding (biz_type, biz_id);
CREATE INDEX IF NOT EXISTS idx_censor_binding_decision ON censor_binding (decision);
CREATE INDEX IF NOT EXISTS idx_censor_binding_review ON censor_binding (review_id);

COMMENT ON TABLE censor_binding IS 'Current moderation state (source of truth)';
COMMENT ON COLUMN censor_binding.review_revision IS 'Increments on each review';

-- ============================================================
-- Table: censor_binding_history
-- Purpose: Historical moderation state changes
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding_history (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    decision        VARCHAR(16) NOT NULL,
    replace_policy  VARCHAR(32) NULL,
    replace_value   VARCHAR(255) NULL,
    violation_ref_id VARCHAR(128) NULL,
    review_revision INT NOT NULL,
    reason_json     JSONB NULL,
    source          VARCHAR(32) NOT NULL,
    reviewer_id     VARCHAR(128) NULL,
    comment         TEXT NULL,
    created_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_binding_history_biz_field ON censor_binding_history (biz_type, biz_id, field, review_revision DESC);
CREATE INDEX IF NOT EXISTS idx_binding_history_source ON censor_binding_history (source, created_at);
CREATE INDEX IF NOT EXISTS idx_binding_history_reviewer ON censor_binding_history (reviewer_id, created_at);
CREATE INDEX IF NOT EXISTS idx_binding_history_created ON censor_binding_history (created_at);

COMMENT ON TABLE censor_binding_history IS 'Historical state changes for audit';
COMMENT ON COLUMN censor_binding_history.source IS 'auto/manual/recheck/policy_upgrade/appeal';
COMMENT ON COLUMN censor_binding_history.reviewer_id IS 'Who made the decision (for manual review)';
COMMENT ON COLUMN censor_binding_history.comment IS 'Reviewer comment or notes';

-- ============================================================
-- Table: violation_snapshot
-- Purpose: Evidence for blocked/review content
-- ============================================================
CREATE TABLE IF NOT EXISTS violation_snapshot (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    content_text    TEXT NULL,
    content_url     TEXT NULL,
    outcome_json    JSONB NOT NULL,
    created_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_violation_biz ON violation_snapshot (biz_type, biz_id);
CREATE INDEX IF NOT EXISTS idx_violation_hash ON violation_snapshot (content_hash);
CREATE INDEX IF NOT EXISTS idx_violation_created ON violation_snapshot (created_at);

COMMENT ON TABLE violation_snapshot IS 'Evidence preservation for blocked content';
