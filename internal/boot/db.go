package boot

import (
	"context"
	"fmt"
	"log/slog"

	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/db"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/schema"
)

func openDBAndMigrate(ctx context.Context, cfg *config.Config, log *slog.Logger) (db.Pool, *tenant.PgConn, error) {
	if cfg.DB.AutoMigrate {
		if err := runMigrations(ctx, cfg, log); err != nil {
			return db.Pool{}, nil, err
		}
	}
	pool, err := db.OpenPool(cfg.DB, log)
	if err != nil {
		return db.Pool{}, nil, err
	}
	if cfg.DB.Driver == db.DriverPostgres {
		if err := schema.RLSGuard(ctx, pool.W, cfg); err != nil {
			_ = pool.Close()
			return db.Pool{}, nil, fmt.Errorf("boot: rls guard: %w", err)
		}
	}
	return pool, tenant.NewPgConn(pool.W), nil
}

func runMigrations(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	migCfg := cfg.DB
	usingMigratorDSN := cfg.DB.Migrator.DSN != ""
	if usingMigratorDSN {
		migCfg.DSN = cfg.DB.Migrator.DSN
		migCfg.Role = cfg.DB.Migrator.Role
	}
	migDB, err := db.Open(migCfg, log)
	if err != nil {
		return fmt.Errorf("boot: open migrator: %w", err)
	}
	defer func() { _ = migDB.Close() }()

	migAwareCfg := *cfg
	migAwareCfg.DB = migCfg
	if err := schema.MigrateUp(ctx, migDB, &migAwareCfg); err != nil {
		return fmt.Errorf("boot: migrate: %w", err)
	}
	if log != nil && usingMigratorDSN {
		log.Info("boot: migrations applied via dedicated migrator connection")
	}
	return nil
}
