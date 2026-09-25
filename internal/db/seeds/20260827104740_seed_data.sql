-- +goose Up

INSERT INTO blocked_domains (domain, reason) VALUES
    ('localhost', 'resolves to loopback address'),
    ('127.0.0.1', 'loopback address'),
    ('::1', 'IPv6 loopback address'),
    ('0.0.0.0', 'unspecified address'),
    ('169.254.169.254', 'cloud metadata endpoint')
ON CONFLICT (domain) DO NOTHING;

INSERT INTO blocked_ip_ranges (cidr, description)
SELECT v.cidr, v.description
FROM (VALUES
    ('127.0.0.0/8'::cidr,    'loopback (IPv4)'),
    ('::1/128'::cidr,        'loopback (IPv6)'),
    ('0.0.0.0/8'::cidr,      'unspecified/this-network'),
    ('10.0.0.0/8'::cidr,     'RFC1918 private'),
    ('172.16.0.0/12'::cidr,  'RFC1918 private'),
    ('192.168.0.0/16'::cidr, 'RFC1918 private'),
    ('100.64.0.0/10'::cidr,  'RFC6598 CGNAT'),
    ('169.254.0.0/16'::cidr, 'link-local (covers cloud metadata 169.254.169.254)'),
    ('fc00::/7'::cidr,       'IPv6 unique local'),
    ('fe80::/10'::cidr,      'IPv6 link-local'),
    ('192.0.0.0/24'::cidr,   'IETF protocol assignments'),
    ('198.18.0.0/15'::cidr,  'benchmarking'),
    ('224.0.0.0/4'::cidr,    'multicast')
) AS v(cidr, description)
WHERE NOT EXISTS (
    SELECT 1 FROM blocked_ip_ranges b WHERE b.cidr = v.cidr
);

-- +goose Down

DELETE FROM blocked_ip_ranges;
DELETE FROM blocked_domains;
