package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver: registers "pgx" with database/sql
	_ "modernc.org/sqlite"             // sqlite driver: registers "sqlite" with database/sql
)

// Open opens the driver-specific *sql.DB, pings it, and applies pool tuning.
func Open(cfg DBConfig, log *slog.Logger) (*sql.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var (
		driver string
		dsn    = cfg.DSN
	)
	switch cfg.Driver {
	case DriverSQLite:
		if err := ensureDirFor(dsn); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(dsn, "file:") && dsn != ":memory:" {
			dsn = fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dsn)
		}
		driver = "sqlite"
	case DriverPostgres:
		driver = "pgx"
	default:
		return nil, fmt.Errorf("db: unknown driver %q", cfg.Driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", driver, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db: ping %s: %w", driver, err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	if log != nil {
		log.Debug("db opened",
			slog.String("driver", driver),
			slog.Int("max_open_conns", cfg.MaxOpenConns),
			slog.Int("max_idle_conns", cfg.MaxIdleConns),
			slog.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
			slog.Duration("conn_max_idle_time", cfg.ConnMaxIdleTime),
		)
	}

	return db, nil
}

func ensureDirFor(path string) error {
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("db: ensure dir %q: %w", dir, err)
	}
	return nil
}
