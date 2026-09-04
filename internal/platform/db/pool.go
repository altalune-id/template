package db

import (
	"context"
	"database/sql"
	"log/slog"
)

// Pool bundles writer (W), reader (R) and maintenance (M) handles. R and M alias W when no separate DSN is configured.
type Pool struct {
	W *sql.DB
	R *sql.DB
	M *sql.DB
}

// OpenPool opens the writer and, for Postgres, any configured reader and maintenance connections.
func OpenPool(ctx context.Context, cfg DBConfig, log *slog.Logger) (Pool, error) {
	writer, err := Open(ctx, cfg, log)
	if err != nil {
		return Pool{}, err
	}
	if cfg.Driver == DriverSQLite {
		if log != nil {
			if cfg.Reader.DSN != "" {
				log.Debug("db: reader DSN ignored for sqlite driver")
			}
			if cfg.Maintenance.DSN != "" {
				log.Debug("db: maintenance DSN ignored for sqlite driver")
			}
		}
		return Pool{W: writer, R: writer, M: writer}, nil
	}

	p := Pool{W: writer, R: writer, M: writer}

	if cfg.Reader.DSN != "" {
		readerCfg := cfg
		readerCfg.DSN = cfg.Reader.DSN
		readerCfg.Role = cfg.Reader.Role
		readerCfg.MaxOpenConns = cfg.Reader.MaxOpenConns
		readerCfg.MaxIdleConns = cfg.Reader.MaxIdleConns
		readerCfg.ConnMaxLifetime = cfg.Reader.ConnMaxLifetime
		readerCfg.ConnMaxIdleTime = cfg.Reader.ConnMaxIdleTime
		reader, rErr := Open(ctx, readerCfg, log)
		if rErr != nil {
			_ = p.Close()
			return Pool{}, rErr
		}
		p.R = reader
	}

	if cfg.Maintenance.DSN != "" {
		maintCfg := cfg
		maintCfg.DSN = cfg.Maintenance.DSN
		maintCfg.Role = cfg.Maintenance.Role
		maintCfg.MaxOpenConns = cfg.Maintenance.MaxOpenConns
		maintCfg.MaxIdleConns = cfg.Maintenance.MaxIdleConns
		maintCfg.ConnMaxLifetime = cfg.Maintenance.ConnMaxLifetime
		maintCfg.ConnMaxIdleTime = cfg.Maintenance.ConnMaxIdleTime
		maint, mErr := Open(ctx, maintCfg, log)
		if mErr != nil {
			_ = p.Close()
			return Pool{}, mErr
		}
		p.M = maint
	}

	return p, nil
}

// Close closes every distinct handle; safe when R or M alias W.
func (p Pool) Close() error {
	if p.R != nil && p.R != p.W {
		_ = p.R.Close()
	}
	if p.M != nil && p.M != p.W && p.M != p.R {
		_ = p.M.Close()
	}
	if p.W != nil {
		return p.W.Close()
	}
	return nil
}
