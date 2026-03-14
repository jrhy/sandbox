CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS thoughts (
  id UUID PRIMARY KEY,
  content TEXT NOT NULL,
  exposure_scope TEXT NOT NULL CHECK (exposure_scope IN ('local_only','remote_ok')),
  user_tags TEXT[] NOT NULL DEFAULT '{}',
  source TEXT NOT NULL DEFAULT '',
  ingest_status TEXT NOT NULL CHECK (ingest_status IN ('pending','ready','failed')),
  failure_reason TEXT NOT NULL DEFAULT '',
  embedding vector(384), -- verified dimension for initial default embedding model
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS thoughts_created_idx ON thoughts (created_at DESC);
CREATE INDEX IF NOT EXISTS thoughts_status_idx ON thoughts (ingest_status, created_at);
CREATE INDEX IF NOT EXISTS thoughts_scope_idx ON thoughts (exposure_scope, created_at DESC);
CREATE INDEX IF NOT EXISTS thoughts_content_fts_idx ON thoughts USING GIN (to_tsvector('english', content));
CREATE INDEX IF NOT EXISTS thoughts_embedding_ivfflat_idx ON thoughts USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
