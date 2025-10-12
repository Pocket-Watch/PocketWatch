ALTER TABLE entries
RENAME created TO created_at;

ALTER TABLE entries
ADD COLUMN last_set_at TIMESTAMP NOT NULL DEFAULT now();

UPDATE entries SET last_set_at = created_at;

