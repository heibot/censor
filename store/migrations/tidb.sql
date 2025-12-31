-- Censor System Database Schema for TiDB
-- TiDB is MySQL-compatible, so this is similar to mysql.sql
-- with TiDB-specific optimizations

-- ============================================================
-- Table: biz_review
-- ============================================================
CREATE TABLE IF NOT EXISTS biz_review (
    id              VARCHAR(64) PRIMARY KEY NONCLUSTERED,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    submitter_id    VARCHAR(128) NOT NULL,
    trace_id        VARCHAR(128) NOT NULL,
    decision        VARCHAR(16) NOT NULL DEFAULT 'pending',
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL,

    INDEX idx_biz (biz_type, biz_id),
    INDEX idx_status (status, decision),
    INDEX idx_created (created_at),
    INDEX idx_submitter (submitter_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: resource_review
-- ============================================================
CREATE TABLE IF NOT EXISTS resource_review (
    id              VARCHAR(64) PRIMARY KEY NONCLUSTERED,
    biz_review_id   VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    content_text    MEDIUMTEXT NULL,
    content_url     TEXT NULL,
    decision        VARCHAR(16) NOT NULL DEFAULT 'pending',
    outcome_json    JSON NULL,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL,

    INDEX idx_biz_review (biz_review_id),
    INDEX idx_hash (content_hash),
    INDEX idx_decision (decision),
    INDEX idx_resource (resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: provider_task
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_task (
    id                  VARCHAR(64) PRIMARY KEY NONCLUSTERED,
    resource_review_id  VARCHAR(64) NOT NULL,
    provider            VARCHAR(32) NOT NULL,
    mode                VARCHAR(8) NOT NULL,
    remote_task_id      VARCHAR(128) NOT NULL,
    done                TINYINT NOT NULL DEFAULT 0,
    result_json         JSON NULL,
    raw_json            JSON NULL,
    created_at          BIGINT NOT NULL,
    updated_at          BIGINT NOT NULL,

    INDEX idx_resource_review (resource_review_id),
    INDEX idx_provider_done (provider, done),
    INDEX idx_remote (provider, remote_task_id),
    INDEX idx_pending (done, mode, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: censor_binding
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding (
    id              VARCHAR(64) PRIMARY KEY NONCLUSTERED,
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

    UNIQUE KEY uq_biz_field (biz_type, biz_id, field),
    INDEX idx_biz (biz_type, biz_id),
    INDEX idx_decision (decision),
    INDEX idx_review (review_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: censor_binding_history
-- ============================================================
CREATE TABLE IF NOT EXISTS censor_binding_history (
    id              VARCHAR(64) PRIMARY KEY NONCLUSTERED,
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
    reason_json     JSON NULL,
    source          VARCHAR(32) NOT NULL,
    created_at      BIGINT NOT NULL,

    INDEX idx_biz_field_rev (biz_type, biz_id, field, review_revision DESC),
    INDEX idx_source (source, created_at),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- Table: violation_snapshot
-- ============================================================
CREATE TABLE IF NOT EXISTS violation_snapshot (
    id              VARCHAR(64) PRIMARY KEY NONCLUSTERED,
    biz_type        VARCHAR(64) NOT NULL,
    biz_id          VARCHAR(128) NOT NULL,
    field           VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    resource_type   VARCHAR(16) NOT NULL,
    content_hash    VARCHAR(128) NOT NULL,
    content_text    MEDIUMTEXT NULL,
    content_url     TEXT NULL,
    outcome_json    JSON NOT NULL,
    created_at      BIGINT NOT NULL,

    INDEX idx_biz (biz_type, biz_id),
    INDEX idx_hash (content_hash),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
