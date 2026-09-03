package db

import (
	"database/sql"
	"log/slog"
)

// Pool bundles writer (W) and reader (R) database handles; R aliases W when no separate reader is configured.
type Pool struct {
	W *sql.DB
	R *sql.DB
}

// OpenPool opens the writer connection and, for Postgres, an optional reader connection when cfg.Reader.DSN is set. SQLite always aliases R to W.
func OpenPool(cfg DBConfig, log *slog.Logger) (Pool, error) {
	writer, err := Open(cfg, log)
	if err != nil {
		return Pool{}, err
	}
	if cfg.Driver == DriverSQLite {
		if cfg.Reader.DSN != "" && log != nil {
			log.Debug("db: reader DSN ignored for sqlite driver")
		}
		return Pool{W: writer, R: writer}, nil
	}
	if cfg.Reader.DSN == "" {
		return Pool{W: writer, R: writer}, nil
	}
	readerCfg := cfg
	readerCfg.DSN = cfg.Reader.DSN
	readerCfg.Role = cfg.Reader.Role
	readerCfg.MaxOpenConns = cfg.Reader.MaxOpenConns
	readerCfg.MaxIdleConns = cfg.Reader.MaxIdleConns
	readerCfg.ConnMaxLifetime = cfg.Reader.ConnMaxLifetime
	readerCfg.ConnMaxIdleTime = cfg.Reader.ConnMaxIdleTime
	reader, err := Open(readerCfg, log)
	if err != nil {
		_ = writer.Close()
		return Pool{}, err
	}
	return Pool{W: writer, R: reader}, nil
}

// Close closes both handles; safe when W and R are the same instance.
func (p Pool) Close() error {
	if p.R != nil && p.R != p.W {
		_ = p.R.Close()
	}
	if p.W != nil {
		return p.W.Close()
	}
	return nil
}
