CREATE TABLE IF NOT EXISTS posts(
    id INTEGER PRIMARY KEY
    , created INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
    , title TEXT NOT NULL DEFAULT ''
    , body TEXT NOT NULL
);
