package db_test

import (
	"testing"

	"altalune.id/template/internal/platform/db"
)

func TestOpen_UnknownDriver(t *testing.T) {
	t.Parallel()
	if _, err := db.Open(db.DBConfig{Driver: "mysql", DSN: "ignored"}, nil); err == nil {
		t.Fatal("Open with unknown driver = nil error, want failure")
	}
}

func TestOpen_EmptyDriver(t *testing.T) {
	t.Parallel()
	if _, err := db.Open(db.DBConfig{DSN: "ignored"}, nil); err == nil {
		t.Fatal("Open with empty driver = nil error, want failure")
	}
}

func TestOpen_EmptyDSN(t *testing.T) {
	t.Parallel()
	if _, err := db.Open(db.DBConfig{Driver: db.DriverSQLite}, nil); err == nil {
		t.Fatal("Open with empty DSN = nil error, want failure")
	}
}

func TestOpen_SQLiteMemory(t *testing.T) {
	t.Parallel()
	sqlDB, err := db.Open(db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:"}, nil)
	if err != nil {
		t.Fatalf("Open(sqlite :memory:) err = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping = %v", err)
	}
}

func TestOpen_SQLiteAppliesPoolTuning(t *testing.T) {
	t.Parallel()
	sqlDB, err := db.Open(db.DBConfig{
		Driver:       db.DriverSQLite,
		DSN:          ":memory:",
		MaxOpenConns: 3,
		MaxIdleConns: 2,
	}, nil)
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if got := sqlDB.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", got)
	}
}

func TestDBConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     db.DBConfig
		wantErr bool
	}{
		{"ok sqlite", db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:"}, false},
		{"ok postgres", db.DBConfig{Driver: db.DriverPostgres, DSN: "postgres://x"}, false},
		{"missing driver", db.DBConfig{DSN: "x"}, true},
		{"unknown driver", db.DBConfig{Driver: "mysql", DSN: "x"}, true},
		{"missing DSN", db.DBConfig{Driver: db.DriverSQLite}, true},
		{"negative MaxOpenConns", db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:", MaxOpenConns: -1}, true},
		{"negative MaxIdleConns", db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:", MaxIdleConns: -1}, true},
		{"negative ConnMaxLifetime", db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:", ConnMaxLifetime: -1}, true},
		{"negative ConnMaxIdleTime", db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:", ConnMaxIdleTime: -1}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
