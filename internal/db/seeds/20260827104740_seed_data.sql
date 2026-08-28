-- +goose Up

INSERT INTO blocked_domains (domain, reason) VALUES
    ('localhost', 'resolves to loopback address'),
    ('127.0.0.1', 'loopback address'),
    ('::1', 'IPv6 loopback address'),
    ('0.0.0.0', 'unspecified address'),
    ('169.254.169.254', 'cloud metadata endpoint');

INSERT INTO blocked_ip_ranges (cidr, description) VALUES
    ('127.0.0.0/8',    'loopback (IPv4)'),
    ('::1/128',        'loopback (IPv6)'),
    ('0.0.0.0/8',      'unspecified/this-network'),
    ('10.0.0.0/8',     'RFC1918 private'),
    ('172.16.0.0/12',  'RFC1918 private'),
    ('192.168.0.0/16', 'RFC1918 private'),
    ('100.64.0.0/10',  'RFC6598 CGNAT'),
    ('169.254.0.0/16', 'link-local (covers cloud metadata 169.254.169.254)'),
    ('fc00::/7',       'IPv6 unique local'),
    ('fe80::/10',      'IPv6 link-local'),
    ('192.0.0.0/24',   'IETF protocol assignments'),
    ('198.18.0.0/15',  'benchmarking'),
    ('224.0.0.0/4',    'multicast');

-- +goose Down

DELETE FROM blocked_ip_ranges;
DELETE FROM blocked_domains;
