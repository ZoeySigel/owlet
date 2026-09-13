CREATE TABLE IF NOT EXISTS users (
 id text PRIMARY KEY, username text UNIQUE NOT NULL, password text NOT NULL,
 admin boolean NOT NULL DEFAULT false, disabled boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS invitations (hash text PRIMARY KEY, used_by text REFERENCES users, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS sessions (hash text PRIMARY KEY, user_id text NOT NULL REFERENCES users, expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS documents (
 id text PRIMARY KEY, owner_id text NOT NULL REFERENCES users, kind text NOT NULL,
 body jsonb NOT NULL, revision integer NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz
);
CREATE INDEX IF NOT EXISTS documents_owner ON documents(owner_id,kind);
CREATE UNIQUE INDEX IF NOT EXISTS one_brand ON documents(owner_id) WHERE kind='brand';
CREATE TABLE IF NOT EXISTS versions (id bigserial PRIMARY KEY, document_id text NOT NULL REFERENCES documents ON DELETE CASCADE, revision integer NOT NULL, body jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(document_id,revision));
CREATE TABLE IF NOT EXISTS assets (id text PRIMARY KEY, owner_id text NOT NULL REFERENCES users, mime text NOT NULL, size bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS quotas (key text PRIMARY KEY, cap bigint NOT NULL, reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0), spent bigint NOT NULL DEFAULT 0 CHECK(spent>=0));
CREATE TABLE IF NOT EXISTS jobs (
 id text PRIMARY KEY, owner_id text NOT NULL REFERENCES users, project_id text REFERENCES documents ON DELETE SET NULL,
 idem text NOT NULL, kind text NOT NULL, input jsonb NOT NULL, state text NOT NULL DEFAULT 'queued',
 reserve bigint NOT NULL, charged bigint NOT NULL DEFAULT 0, day_key text NOT NULL, month_key text NOT NULL,
 result jsonb NOT NULL DEFAULT '{}', error text NOT NULL DEFAULT '', cancel_requested boolean NOT NULL DEFAULT false,
 dispatched boolean NOT NULL DEFAULT false, settled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(owner_id,idem)
);
CREATE INDEX IF NOT EXISTS jobs_queue ON jobs(state,created_at);
CREATE TABLE IF NOT EXISTS events (id bigserial PRIMARY KEY, job_id text NOT NULL REFERENCES jobs ON DELETE CASCADE, body jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS ledger (job_id text PRIMARY KEY REFERENCES jobs, amount bigint NOT NULL, note text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS settings (key text PRIMARY KEY, value text NOT NULL);
INSERT INTO settings(key,value) VALUES('paused','false') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS audit (id bigserial PRIMARY KEY, actor text NOT NULL, action text NOT NULL, target text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
