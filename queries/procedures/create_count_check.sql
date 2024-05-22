DROP PROCEDURE IF EXISTS CountIfExists;

CREATE PROCEDURE CountIfExists(tableName TEXT, colName TEXT, len TEXT)
BEGIN
	DECLARE columnExists INT;

	-- check if column exists
	SELECT COUNT(*) INTO columnExists FROM INFORMATION_SCHEMA.COLUMNS
    WHERE table_name = tableName
    AND table_schema = DATABASE()
    AND column_name =  colName;

	IF columnExists > 0 THEN
		SET @s = CONCAT('SELECT COUNT(*) FROM ', tableName, ' WHERE LENGTH(', colName, ') > ', len);

		PREPARE stmt1 FROM @s;
		EXECUTE stmt1;
		DEALLOCATE PREPARE stmt1;
	ELSE
		SELECT 0;
	END IF;
END;
