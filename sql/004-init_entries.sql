CREATE TABLE entries (
    id          BIGSERIAL PRIMARY KEY,
    url         TEXT      NOT NULL,
    title       TEXT      NOT NULL,
    user_id     BIGINT    NOT NULL,
    use_proxy   BOOLEAN   NOT NULL,
    referer_url TEXT      NOT NULL,
    source_url  TEXT      NOT NULL,
    thumbnail   TEXT      NOT NULL,
    created     TIMESTAMP NOT NULL
);

CREATE TABLE subtitles (
    id       BIGSERIAL PRIMARY KEY,
    entry_id BIGINT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    name     TEXT      NOT NULL,
    url      TEXT      NOT NULL,
    shift    DOUBLE    PRECISION NOT NULL
);

CREATE TABLE current_entry (
    id       BIGSERIAL PRIMARY KEY,
    entry_id BIGINT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE history (
    id       BIGSERIAL PRIMARY KEY,
    entry_id BIGINT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE playlist (
    id       BIGSERIAL PRIMARY KEY,
    entry_id BIGINT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
