package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	statementTimeoutInSeconds = 60 * 5 // 5 minutes
)

type DB struct {
	databaseName string
	db           *sql.DB
	conn         *sql.Conn
}

func NewStore(dbType string, dataSource string) (*DB, error) {
	switch dbType {
	case "mysql":
		return openMySQL(dataSource)
	case "postgres":
		return openPostgres(dataSource)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
}

func (db *DB) GetDB() *sql.DB {
	return db.db
}

func (db *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), statementTimeoutInSeconds*time.Second)
	defer cancel()

	return db.conn.PingContext(ctx)
}

func (db *DB) Close() error {
	if db.conn != nil {
		if err := db.conn.Close(); err != nil {
			return fmt.Errorf("could not close connection: %w", err)
		}
	}

	if db.db != nil {
		if err := db.db.Close(); err != nil {
			return fmt.Errorf("could not close DB: %w", err)
		}
	}

	return nil
}

func (db *DB) RunSelectCountQuery(ctx context.Context, query string) (int, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, query).Scan(&count)

	return count, err
}

func (db *DB) ExecQuery(ctx context.Context, query string) error {
	_, err := db.conn.ExecContext(ctx, query)

	return err
}
