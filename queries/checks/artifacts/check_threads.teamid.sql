SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE table_name = 'Threads' AND table_schema = DATABASE() AND column_name = 'TeamId';
