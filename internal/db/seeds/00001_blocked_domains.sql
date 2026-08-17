-- +goose Up
INSERT INTO blocked_domains (domain, reason) VALUES
    ('localhost', 'resolves to loopback address'),
    ('127.0.0.1', 'loopback address'),
    ('::1', 'IPv6 loopback address'),
    ('0.0.0.0', 'unspecified address'),
    ('169.254.169.254', 'cloud metadata endpoint');

-- +goose Down
DELETE FROM blocked_domains WHERE id IN (1, 2, 3, 4, 5);
