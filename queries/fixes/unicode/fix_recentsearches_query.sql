SET @preparedStatement = (SELECT IF(
 (
     SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
     WHERE table_name = 'RecentSearches'
     AND table_schema = DATABASE()
     AND column_name = 'Query'
 ) > 0,
 'UPDATE RecentSearches SET Query = REPLACE(Query, \'\\\\u0000\', \'\') WHERE Query LIKE \'%\\u0000%\';',
 'SELECT 1'
));

PREPARE updateIfExists FROM @preparedStatement;
EXECUTE updateIfExists;
DEALLOCATE PREPARE updateIfExists;
