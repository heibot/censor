-- Censor System Database Schema for MySQL
-- Execute this file to initialize the database tables.

-- ============================================================
-- Table: biz_review
-- Purpose: Stores business-level review records
-- One record per business object review request
-- ============================================================
CREATE TABLE IF NOT EXISTS biz_review (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL COMMENT 'Business type: user_avatar, note_body, etc.',
    biz_id          VARCHAR(128) NOT NULL COMMENT 'Business object ID: userID, noteID, etc.',
    field           VARCHAR(64) NOT NULL COMMENT 'Specific field: title, body, avatar, etc.',
    submitter_id    VARCHAR(128) NOT NULL COMMENT 'Who submitted the content',
    trace_id        VARCHAR(128) NOT NULL COMMENT 'Request trace ID for debugging',
    decision        VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT 'pass/review/block/error/pending',
    status          VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT 'pending/running/done/failed/canceled',
    created_at      BIGINT NOT NULL COMMENT 'Unix timestamp in milliseconds',
    updated_at      BIGINT NOT NULL COMMENT 'Unix timestamp in milliseconds',

    INDEX idx_biz (biz_type, biz_id),
    INDEX idx_status (status, decision),
    INDEX idx_created (created_at),
    INDEX idx_submitter (submitter_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: resource_review
-- Purpose: Stores resource-level review records
-- One record per resource within a business review
-- ============================================================
CREATE TABLE IF NOT EXISTS resource_review (
    id              VARCHAR(64) PRIMARY KEY,
    biz_review_id   VARCHAR(64) NOT NULL COMMENT 'Reference to biz_review.id',
    resource_id     VARCHAR(128) NOT NULL COMMENT 'Unique resource identifier',
    resource_type   VARCHAR(16) NOT NULL COMMENT 'text/image/video',
    content_hash    VARCHAR(128) NOT NULL COMMENT 'SHA256 hash for deduplication',
    content_text    MEDIUMTEXT NULL COMMENT 'Text content (for text type)',
    content_url     TEXT NULL COMMENT 'URL for image/video',
    decision        VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT 'pass/review/block/error/pending',
    outcome_json    JSON NULL COMMENT 'FinalOutcome as JSON',
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL,

    INDEX idx_biz_review (biz_review_id),
    INDEX idx_hash (content_hash),
    INDEX idx_decision (decision),
    INDEX idx_resource (resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: provider_task
-- Purpose: Stores provider task records
-- One record per provider API call
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_task (
    id                  VARCHAR(64) PRIMARY KEY,
    resource_review_id  VARCHAR(64) NOT NULL COMMENT 'Reference to resource_review.id',
    provider            VARCHAR(32) NOT NULL COMMENT 'aliyun/huawei/tencent/manual',
    mode                VARCHAR(8) NOT NULL COMMENT 'sync/async',
    remote_task_id      VARCHAR(128) NOT NULL COMMENT 'Provider-side task ID',
    done                TINYINT NOT NULL DEFAULT 0 COMMENT '0=pending, 1=done',
    result_json         JSON NULL COMMENT 'ReviewResult as JSON',
    raw_json            JSON NULL COMMENT 'Raw provider response (trimmed)',
    created_at          BIGINT NOT NULL,
    updated_at          BIGINT NOT NULL,

    INDEX idx_resource_review (resource_review_id),
    INDEX idx_provider_done (provider, done),
    INDEX idx_remote (provider, remote_task_id),
    INDEX idx_pending (done, mode, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: censor_binding
-- Purpose: Stores current moderation state for business fields
-- One record per (biz_type, biz_id, field) combination
-- This is the "source of truth" for current state
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    review_id       VARCHAR(128) NOT NULL COMMENT 'Reference to biz_review.id',
    decision        VARCHAR(16) NOT NULL COMMENT 'pass/review/block',
    replace_policy  VARCHAR(32) NULL COMMENT 'none/default_value/mask',
    replace_value   VARCHAR(255) NULL COMMENT 'Replacement value if applicable',
    violation_ref_id VARCHAR(128) NULL COMMENT 'Reference to violation_snapshot.id',
    review_revision INT NOT NULL DEFAULT 1 COMMENT 'Increments on each review',
    updated_at      BIGINT NOT NULL,

    UNIQUE KEY uq_biz_field (biz_type, biz_id, field),
    INDEX idx_biz (biz_type, biz_id),
    INDEX idx_decision (decision),
    INDEX idx_review (review_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: censor_binding_history
-- Purpose: Stores historical moderation state changes
-- One record per state change (for audit, appeal, analytics)
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
    reason_json     JSON NULL COMMENT 'UnifiedViolation[] as JSON',
    source          VARCHAR(32) NOT NULL COMMENT 'auto/manual/recheck/policy_upgrade/appeal',
    reviewer_id     VARCHAR(128) NULL COMMENT 'Who made the decision (for manual review)',
    comment         TEXT NULL COMMENT 'Reviewer comment or notes',
    created_at      BIGINT NOT NULL,

    INDEX idx_biz_field_rev (biz_type, biz_id, field, review_revision DESC),
    INDEX idx_source (source, created_at),
    INDEX idx_reviewer (reviewer_id, created_at),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: violation_snapshot
-- Purpose: Stores evidence for blocked/review content
-- Preserves original content for audit, appeal, legal
-- ============================================================
CREATE TABLE IF NOT EXISTS violation_snapshot (
    id              VARCHAR(64) PRIMARY KEY,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    content_text    MEDIUMTEXT NULL COMMENT 'Original text content',
    content_url     TEXT NULL COMMENT 'Original URL (may expire)',
    outcome_json    JSON NOT NULL COMMENT 'FinalOutcome with reasons',
    created_at      BIGINT NOT NULL,

    INDEX idx_biz (biz_type, biz_id),
    INDEX idx_hash (content_hash),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
