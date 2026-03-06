-- +goose Up
-- Issue #234: Partitions starting from March 2026 (first release month) through December 2028

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_03
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_04
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_05
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_06
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_07
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_08
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_09
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_10
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2026_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_01
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_02
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_03
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_04
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_05
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_06
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_07
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-07-01') TO ('2027-08-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_08
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-08-01') TO ('2027-09-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_09
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-09-01') TO ('2027-10-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_10
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-10-01') TO ('2027-11-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-11-01') TO ('2027-12-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2027_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-12-01') TO ('2028-01-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_01
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_02
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_03
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_04
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_05
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_06
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_07
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_08
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_09
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_10
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_events_2028_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_events_2026_03;
DROP TABLE IF EXISTS audit_events_2026_04;
DROP TABLE IF EXISTS audit_events_2026_05;
DROP TABLE IF EXISTS audit_events_2026_06;
DROP TABLE IF EXISTS audit_events_2026_07;
DROP TABLE IF EXISTS audit_events_2026_08;
DROP TABLE IF EXISTS audit_events_2026_09;
DROP TABLE IF EXISTS audit_events_2026_10;
DROP TABLE IF EXISTS audit_events_2026_11;
DROP TABLE IF EXISTS audit_events_2026_12;
DROP TABLE IF EXISTS audit_events_2027_01;
DROP TABLE IF EXISTS audit_events_2027_02;
DROP TABLE IF EXISTS audit_events_2027_03;
DROP TABLE IF EXISTS audit_events_2027_04;
DROP TABLE IF EXISTS audit_events_2027_05;
DROP TABLE IF EXISTS audit_events_2027_06;
DROP TABLE IF EXISTS audit_events_2027_07;
DROP TABLE IF EXISTS audit_events_2027_08;
DROP TABLE IF EXISTS audit_events_2027_09;
DROP TABLE IF EXISTS audit_events_2027_10;
DROP TABLE IF EXISTS audit_events_2027_11;
DROP TABLE IF EXISTS audit_events_2027_12;
DROP TABLE IF EXISTS audit_events_2028_01;
DROP TABLE IF EXISTS audit_events_2028_02;
DROP TABLE IF EXISTS audit_events_2028_03;
DROP TABLE IF EXISTS audit_events_2028_04;
DROP TABLE IF EXISTS audit_events_2028_05;
DROP TABLE IF EXISTS audit_events_2028_06;
DROP TABLE IF EXISTS audit_events_2028_07;
DROP TABLE IF EXISTS audit_events_2028_08;
DROP TABLE IF EXISTS audit_events_2028_09;
DROP TABLE IF EXISTS audit_events_2028_10;
DROP TABLE IF EXISTS audit_events_2028_11;
DROP TABLE IF EXISTS audit_events_2028_12;
-- +goose StatementEnd
