-- +goose Up
INSERT INTO blocked_domains (id, domain, reason) VALUES
    (1, 'localhost', 'resolves to loopback address'),
    (2, '127.0.0.1', 'loopback address'),
    (3, '::1', 'IPv6 loopback address'),
    (4, '0.0.0.0', 'unspecified address'),
    (5, '169.254.169.254', 'cloud metadata endpoint');

-- +goose Down
DELETE FROM blocked_domains WHERE id IN (1, 2, 3, 4, 5);
