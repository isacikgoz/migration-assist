package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

func openMySQL(dataSource string) (*DB, error) {
	sanitizedDataSource, err := appendMultipleStatementsFlag(dataSource)
	if err != nil {
		return nil, fmt.Errorf("could not parse DSN: %w", err)
	}

	db, err := sql.Open("mysql", sanitizedDataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection with the database: %w", err)
	}

	dbName, err := extractMySQLDatabaseNameFromURL(sanitizedDataSource)
	if err != nil {
		return nil, fmt.Errorf("could not parse database name: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to grab connection to the database")
	}

	return &DB{
		db:           db,
		conn:         conn,
		databaseName: dbName,
	}, nil
}

func extractMySQLDatabaseNameFromURL(conn string) (string, error) {
	cfg, err := mysql.ParseDSN(conn)
	if err != nil {
		return "", err
	}

	return cfg.DBName, nil
}

func appendMultipleStatementsFlag(dataSource string) (string, error) {
	config, err := mysql.ParseDSN(dataSource)
	if err != nil {
		return "", err
	}

	if config.Params == nil {
		config.Params = map[string]string{}
	}

	config.Params["multiStatements"] = "true"
	return config.FormatDSN(), nil
}
