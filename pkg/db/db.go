package db

import (
	"database/sql"
	"os"

	_ "github.com/lib/pq"
)

func Connect() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://cc:ccpass@localhost:5432/creditcard?sslmode=disable"
	}
	return sql.Open("postgres", dsn)
}
