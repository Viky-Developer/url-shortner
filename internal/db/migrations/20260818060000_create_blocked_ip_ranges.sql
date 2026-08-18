-- +goose Up

CREATE TABLE blocked_ip_ranges (
    cidr        CIDR NOT NULL,
    description TEXT NOT NULL
);

-- +goose Down

DROP TABLE IF EXISTS blocked_ip_ranges;
