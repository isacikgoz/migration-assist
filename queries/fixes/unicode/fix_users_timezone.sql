SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'Users'
     AND table_schema = DATABASE()
     AND column_name = 'Timezone'
 ) > 0,
 'UPDATE Users SET Timezone = REPLACE(Timezone, \'\\\\u0000\', \'\') WHERE Timezone LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
