SET @preparedStatement = (SELECT IF(
    (
        SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
        WHERE table_name = 'schema_migrations'
        AND table_schema = DATABASE()
    ) > 0,
    'DROP TABLE schema_migrations;',
    'SELECT 1'
));

PREPARE dropIfExists FROM @preparedStatement;
EXECUTE dropIfExists;
DEALLOCATE PREPARE dropIfExists;
