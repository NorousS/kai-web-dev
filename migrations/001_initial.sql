-- +goose Up
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL CHECK (char_length(username) BETWEEN 3 AND 50),
    email VARCHAR(100) NOT NULL,
    password TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    is_blocked BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE UNIQUE INDEX users_email_unique ON users (lower(email));

CREATE TABLE tasks (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(150) NOT NULL CHECK (char_length(trim(title)) BETWEEN 1 AND 150),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','in_progress','completed')),
    priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high')),
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tasks_user_status_created ON tasks(user_id,status,created_at DESC);

CREATE TABLE comments (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text TEXT NOT NULL CHECK (char_length(trim(text))>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX comments_task_created ON comments(task_id,created_at);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE INDEX sessions_expiry ON sessions(expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE comments;
DROP TABLE tasks;
DROP TABLE users;
