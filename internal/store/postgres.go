package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
)

func openPostgres(dataSource string) (*DB, error) {
	db, err := sql.Open("postgres", dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection with the database: %w", err)
	}

	dbName, err := extractPostgresDatabaseNameFromURL(dataSource)
	if err != nil {
		return nil, fmt.Errorf("could not parse database name: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to grab connection to the database: %w", err)
	}

	return &DB{
		db:           db,
		conn:         conn,
		databaseName: dbName,
	}, nil
}

func extractPostgresDatabaseNameFromURL(conn string) (string, error) {
	uri, err := url.Parse(conn)
	if err != nil {
		return "", err
	}

	return uri.Path[1:], nil
}
