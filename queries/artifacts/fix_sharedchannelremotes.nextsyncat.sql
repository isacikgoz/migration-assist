SET @preparedStatement = (SELECT IF(
    (
        SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
        WHERE table_name = 'SharedChannelRemotes'
        AND table_schema = DATABASE()
        AND COLUMN_NAME = 'NextSyncAt'
    ) > 0,
    'ALTER TABLE SharedChannelRemotes DROP COLUMN NextSyncAt;',
    'SELECT 1'
));

PREPARE alterIfExists FROM @preparedStatement;
EXECUTE alterIfExists;
DEALLOCATE PREPARE alterIfExists;
