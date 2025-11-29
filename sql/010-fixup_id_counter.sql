SELECT setval(
    pg_get_serial_sequence('users', 'id'), COALESCE(MAX(id), 0) + 1, false
) FROM users;

SELECT setval(
    pg_get_serial_sequence('entries', 'id'), COALESCE(MAX(id), 0) + 1, false
) FROM entries;

SELECT setval(
    pg_get_serial_sequence('subtitles', 'id'), COALESCE(MAX(id), 0) + 1, false
) FROM subtitles;

SELECT setval(
    pg_get_serial_sequence('messages', 'id'), COALESCE(MAX(id), 0) + 1, false
) FROM messages;
