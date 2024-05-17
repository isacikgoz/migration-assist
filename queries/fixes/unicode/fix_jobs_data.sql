SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'Jobs'
     AND table_schema = DATABASE()
     AND column_name = 'Data'
 ) > 0,
 'UPDATE Jobs SET Data = REPLACE(Data, \'\\\\u0000\', \'\') WHERE Data LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
