CREATE TABLE messages (
    id         BIGSERIAL PRIMARY KEY,
    content    TEXT      NOT NULL,
    created_at BIGINT    NOT NULL,
    edited_at  BIGINT    NOT NULL,
    user_id    BIGINT    NOT NULL
);
