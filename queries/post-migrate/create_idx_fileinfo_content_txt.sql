CREATE INDEX IF NOT EXISTS idx_fileinfo_content_txt ON fileinfo USING gin(to_tsvector('english', content));
