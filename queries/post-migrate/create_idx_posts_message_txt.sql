CREATE INDEX IF NOT EXISTS idx_posts_message_txt ON posts USING gin(to_tsvector('english', message));
