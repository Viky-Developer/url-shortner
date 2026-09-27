-- +goose Up
ALTER TABLE users
    ALTER COLUMN password_hash DROP NOT NULL,
    ADD COLUMN has_password BOOLEAN NOT NULL DEFAULT TRUE,
    ADD CONSTRAINT users_password_state_check CHECK (
        (has_password AND password_hash IS NOT NULL)
        OR (NOT has_password AND password_hash IS NULL)
    );

COMMENT ON COLUMN users.has_password IS
    'Whether password_hash contains a usable local login credential.';

ALTER TABLE sessions
    ADD COLUMN auth_provider VARCHAR(16) NOT NULL DEFAULT 'SYSTEM',
    ADD CONSTRAINT sessions_auth_provider_check CHECK (
        auth_provider IN ('GOOGLE', 'SYSTEM')
    );

-- +goose Down
ALTER TABLE sessions
    DROP CONSTRAINT sessions_auth_provider_check,
    DROP COLUMN auth_provider;

ALTER TABLE users
    DROP CONSTRAINT users_password_state_check;

UPDATE users
SET password_hash = ''
WHERE password_hash IS NULL;

ALTER TABLE users
    ALTER COLUMN password_hash SET NOT NULL,
    DROP COLUMN has_password;
