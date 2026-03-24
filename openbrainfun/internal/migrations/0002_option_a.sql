alter table thoughts add column if not exists embedding_status text not null default 'pending';
alter table thoughts add column if not exists embedding_error text not null default '';
alter table thoughts add column if not exists embedding_fingerprint text not null default '';
alter table thoughts add column if not exists embedding_updated_at timestamptz;
alter table thoughts add column if not exists metadata_status text not null default 'pending';
alter table thoughts add column if not exists metadata_error text not null default '';
alter table thoughts add column if not exists metadata_model text not null default '';
alter table thoughts add column if not exists metadata_fingerprint text not null default '';
alter table thoughts add column if not exists metadata_updated_at timestamptz;

update thoughts
set embedding_status = case
        when embedding is not null and ingest_status = 'ready' then 'ready'
        when ingest_status = 'failed' then 'failed'
        else 'pending'
    end,
    embedding_error = case
        when ingest_status = 'failed' then coalesce(ingest_error, '')
        else coalesce(embedding_error, '')
    end,
    embedding_fingerprint = case
        when coalesce(embedding_fingerprint, '') <> '' then embedding_fingerprint
        else coalesce(embedding_model, '')
    end,
    embedding_updated_at = case
        when embedding_updated_at is not null then embedding_updated_at
        when embedding is not null and ingest_status = 'ready' then updated_at
        else embedding_updated_at
    end,
    metadata_status = case
        when ingest_status = 'ready' then 'ready'
        else 'pending'
    end,
    metadata_error = coalesce(metadata_error, ''),
    metadata_model = coalesce(metadata_model, ''),
    metadata_fingerprint = coalesce(metadata_fingerprint, ''),
    metadata_updated_at = case
        when metadata_updated_at is not null then metadata_updated_at
        when ingest_status = 'ready' then updated_at
        else metadata_updated_at
    end;
