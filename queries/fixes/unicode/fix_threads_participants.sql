SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'Threads'
     AND table_schema = DATABASE()
     AND column_name = 'Participants'
 ) > 0,
 'UPDATE Threads SET Participants = REPLACE(Participants, \'\\\\u0000\', \'\') WHERE Participants LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
