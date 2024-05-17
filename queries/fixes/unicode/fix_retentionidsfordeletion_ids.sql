SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'RetentionIdsForDeletion'
     AND table_schema = DATABASE()
     AND column_name = 'Ids'
 ) > 0,
 'UPDATE RetentionIdsForDeletion SET Ids = REPLACE(Ids, \'\\\\u0000\', \'\') WHERE Ids LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
