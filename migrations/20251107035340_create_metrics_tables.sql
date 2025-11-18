-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS gauge (
    name TEXT PRIMARY KEY,
    value DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS counter (
    name TEXT PRIMARY KEY,
    value BIGINT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gauge;
DROP TABLE IF EXISTS counter;
-- +goose StatementEnd