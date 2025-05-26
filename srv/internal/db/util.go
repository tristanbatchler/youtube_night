package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Embed the database schema to be used when creating the database tables
//
//go:embed config/schema.sql
var schemaGenSql string

func GenSchema(dbPool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := dbPool.Exec(ctx, schemaGenSql)
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	return nil
}

func ErrorHasCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == code
	}

	return false
}
