SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'Sessions'
     AND table_schema = DATABASE()
     AND column_name = 'Props'
 ) > 0,
 'UPDATE Sessions SET Props = REPLACE(Props, \'\\\\u0000\', \'\') WHERE Props LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
