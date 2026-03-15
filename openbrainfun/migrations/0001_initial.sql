create extension if not exists pgcrypto;
create extension if not exists vector;

create table users (
    id uuid primary key default gen_random_uuid(),
    username text not null unique,
    password_hash text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    disabled_at timestamptz
);

create table web_sessions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    session_token_hash text not null,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    last_seen_at timestamptz not null default now()
);

create table mcp_tokens (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    token_hash text not null,
    label text not null,
    created_at timestamptz not null default now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);

create table thoughts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    content text not null,
    exposure_scope text not null,
    user_tags text[] not null default '{}',
    metadata jsonb not null default '{}'::jsonb,
    embedding vector(384),
    embedding_model text,
    ingest_status text not null,
    ingest_error text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
