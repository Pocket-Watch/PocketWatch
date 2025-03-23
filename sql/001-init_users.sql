CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL    PRIMARY KEY,
    username    VARCHAR(255) NOT NULL,
    avatar_path VARCHAR(255),
    token       VARCHAR(255) NOT NULL,
    created     TIMESTAMP    NOT NULL,
    last_update TIMESTAMP    NOT NULL
)
