package db

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultDBURL = "postgresql://postgres:postgres@localhost:5432/simple_bank?sslmode=disable"

// testQueries and testDB are shared by every test in this package.
var (
	testQueries *Queries
	testDB      *pgxpool.Pool
)

// TestMain connects to the database once before the package's tests run
// and closes the connection after they finish.
func TestMain(m *testing.M) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDBURL
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal("cannot create db pool: ", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		log.Fatal("cannot connect to db: ", err)
	}

	testDB = pool
	testQueries = New(pool)

	code := m.Run()

	pool.Close()
	os.Exit(code)
}
