CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    avatar_path TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS gangs (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    entry_password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users_gangs (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    gang_id INTEGER NOT NULL REFERENCES gangs(id) ON DELETE CASCADE,
    isHost BOOLEAN NOT NULL DEFAULT FALSE,
    associated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, gang_id)
);

CREATE TABLE IF NOT EXISTS videos (
    video_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    thumbnail_url TEXT NOT NULL,
    channel_name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS video_submissions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    gang_id INTEGER NOT NULL REFERENCES gangs(id) ON DELETE CASCADE,
    video_id TEXT NOT NULL REFERENCES videos(video_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, gang_id, video_id)
);