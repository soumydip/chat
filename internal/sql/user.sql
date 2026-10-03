
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_cron;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- users

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      CITEXT       NOT NULL UNIQUE,   
    full_name     VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

DROP TRIGGER IF EXISTS trg_users_updated ON users;
CREATE TRIGGER trg_users_updated BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();



CREATE TABLE IF NOT EXISTS user_sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token_hash VARCHAR(64)  NOT NULL UNIQUE, 
    expires_at         TIMESTAMPTZ  NOT NULL DEFAULT (NOW() + INTERVAL '30 days'),
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires  ON user_sessions(expires_at);

-- Expired session cleanup (pg_cron):
SELECT cron.schedule('cleanup-sessions', '0 * * * *',
  $$DELETE FROM user_sessions WHERE expires_at < NOW()$$);


-- conversations

CREATE TABLE IF NOT EXISTS conversations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    is_group        BOOLEAN      NOT NULL DEFAULT FALSE,
    dm_key          TEXT,                   
    name            VARCHAR(255),             
    description     TEXT,
    avatar_url      TEXT,
    last_message_at TIMESTAMPTZ,        
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_conversation_kind CHECK (
        (is_group = TRUE  AND dm_key IS NULL) OR
        (is_group = FALSE AND dm_key IS NOT NULL)
    )
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_conversations_dm_key
    ON conversations(dm_key) WHERE dm_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_last_message
    ON conversations(last_message_at DESC NULLS LAST);

DROP TRIGGER IF EXISTS trg_conversations_updated ON conversations;
CREATE TRIGGER trg_conversations_updated BEFORE UPDATE ON conversations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();


-- conversation_members

CREATE TABLE IF NOT EXISTS conversation_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            VARCHAR(10) NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    last_read_at    TIMESTAMPTZ,               -- unread count: created_at > last_read_at
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (conversation_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_conv_members_user ON conversation_members(user_id);


-- messages

CREATE TABLE IF NOT EXISTS messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       UUID        REFERENCES users(id) ON DELETE SET NULL,
    reply_to_id     UUID        REFERENCES messages(id) ON DELETE SET NULL,
    content         TEXT        NOT NULL,
    is_forwarded    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ,               -- NULL = kokhono edit hoyni
    deleted_at      TIMESTAMPTZ                -- soft delete
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created
    ON messages(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_reply_to
    ON messages(reply_to_id) WHERE reply_to_id IS NOT NULL;

-- msg edit auto update updated_at
CREATE OR REPLACE FUNCTION set_message_edited() RETURNS trigger AS $$
BEGIN
    IF NEW.content IS DISTINCT FROM OLD.content THEN
        NEW.updated_at = NOW();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_messages_edited ON messages;
CREATE TRIGGER trg_messages_edited BEFORE UPDATE ON messages
FOR EACH ROW EXECUTE FUNCTION set_message_edited();

-- conversation last_message_at update on new message
CREATE OR REPLACE FUNCTION bump_conversation_last_message() RETURNS trigger AS $$
BEGIN
    UPDATE conversations
    SET last_message_at = NEW.created_at
    WHERE id = NEW.conversation_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_messages_bump_conversation ON messages;
CREATE TRIGGER trg_messages_bump_conversation AFTER INSERT ON messages
FOR EACH ROW EXECUTE FUNCTION bump_conversation_last_message();


-- attachments

CREATE TABLE IF NOT EXISTS attachments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    file_name  TEXT        NOT NULL,
    file_type  VARCHAR(127) NOT NULL,           -- MIME type
    file_size  BIGINT      NOT NULL CHECK (file_size >= 0),
    file_url   TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_attachments_message_id ON attachments(message_id);