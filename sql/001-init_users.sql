CREATE TABLE users (
    id          BIGSERIAL    PRIMARY KEY,
    username    VARCHAR(255) NOT NULL,
    avatar_path VARCHAR(255),
    token       VARCHAR(255) NOT NULL,
    created_at  TIMESTAMP    NOT NULL,
    last_update TIMESTAMP    NOT NULL,
    last_online TIMESTAMP    NOT NULL
)
