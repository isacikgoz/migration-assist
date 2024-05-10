SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE table_name = 'SharedChannelRemotes' AND table_schema = DATABASE() AND COLUMN_NAME = 'NextSyncAt';
