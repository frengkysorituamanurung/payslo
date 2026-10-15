-- U4: rejection reason on approval decisions. Applied idempotently by
-- db::open (duplicate-column errors are ignored for databases created
-- before this migration existed).
ALTER TABLE approval_requests ADD COLUMN reason TEXT NOT NULL DEFAULT '';
