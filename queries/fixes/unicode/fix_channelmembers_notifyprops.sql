SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'ChannelMembers'
     AND table_schema = DATABASE()
     AND column_name = 'NotifyProps'
 ) > 0,
 'UPDATE ChannelMembers SET NotifyProps = REPLACE(NotifyProps, \'\\\\u0000\', \'\') WHERE NotifyProps LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
