package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"slices"

	"github.com/isacikgoz/migration-assist/internal/logger"
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
		dbType:       "postgres",
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

func (db *DB) CheckPostgresDefaultSchema(ctx context.Context, schema string, logger logger.LogInterface) error {
	rows, err := db.db.QueryContext(ctx, "SHOW search_path")
	if err != nil {
		return fmt.Errorf("could not determine the search_path: %w", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var s string
		err := rows.Scan(&s)
		if err != nil {
			return fmt.Errorf("could not scan the schema for search_path: %w", err)
		}
		schemas = append(schemas, s)
	}
	if len(schemas) == 0 {
		return fmt.Errorf("no value available for search_path")
	} else if _, ok := slices.BinarySearch(schemas, schema); !ok {
		logger.Printf("could not find the default schema %q in search_path, consider setting it from the postgresql console\n", schema)
		err := db.ExecQuery(ctx, fmt.Sprintf("SELECT pg_catalog.set_config('search_path', '\"$user\", %s', false)", schema))
		if err != nil {
			return fmt.Errorf("could not set search_path for the session: %w", err)
		}
		logger.Printf("search_path is set to %q for the currrent session\n", schema)
	}

	return nil
}
