-- +goose Up
-- 1. Users Table (Optional - for account management)
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ NULL
);

-- 2. Sessions Table (Device login tracking)
CREATE TABLE sessions (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_type    VARCHAR(50),
    device_name    VARCHAR(255),
    ip_address     INET,
    user_agent     TEXT,
    logged_in_at   TIMESTAMPTZ DEFAULT NOW(),
    last_active_at TIMESTAMPTZ DEFAULT NOW(),
    is_active      BOOLEAN DEFAULT TRUE
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

-- 3. URLs Table (Core redirect lookup)
CREATE TABLE urls (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    short_code    VARCHAR(16) UNIQUE NOT NULL,
    original_url  TEXT NOT NULL,
    is_custom     BOOLEAN DEFAULT FALSE,
    expires_at    TIMESTAMPTZ NULL,
    is_active     BOOLEAN DEFAULT TRUE,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_urls_short_code ON urls(short_code);

-- 4. Click Analytics Table (Kept separate to protect read performance)
CREATE TABLE click_logs (
    id          BIGSERIAL PRIMARY KEY,
    url_id      BIGINT NOT NULL REFERENCES urls(id) ON DELETE CASCADE,
    clicked_at  TIMESTAMPTZ DEFAULT NOW(),
    ip_address  INET,
    user_agent  TEXT,
    referrer    TEXT
);

CREATE INDEX idx_click_logs_url_id ON click_logs(url_id);
CREATE INDEX idx_click_logs_url_clicked ON click_logs(url_id, clicked_at);

-- +goose Down
DROP TABLE IF EXISTS click_logs;
DROP TABLE IF EXISTS urls;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;