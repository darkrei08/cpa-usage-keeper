package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConfig_DatabaseDriverSelection(t *testing.T) {
	t.Setenv("CPA_BASE_URL", "http://127.0.0.1:8317")
	t.Setenv("CPA_MANAGEMENT_KEY", "test-key")
	t.Setenv("AUTH_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_DRIVER", "")

	// 1. Default fallback to sqlite
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}
	if cfg.DatabaseDriver != "sqlite" {
		t.Errorf("expected default driver sqlite, got %s", cfg.DatabaseDriver)
	}

	// 2. Explicit DATABASE_URL triggers postgres
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/keeper_test")
	t.Setenv("DB_DRIVER", "")
	cfgPostgres, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("failed to load postgres config: %v", err)
	}
	if cfgPostgres.DatabaseDriver != "postgres" {
		t.Errorf("expected driver postgres, got %s", cfgPostgres.DatabaseDriver)
	}
	if cfgPostgres.DatabaseDSN != "postgres://user:pass@localhost:5432/keeper_test" {
		t.Errorf("unexpected database DSN: %s", cfgPostgres.DatabaseDSN)
	}
}

func TestRepository_DatabaseHasTablesDialectAgnostic(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite memory: %v", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	writer, reader, err := repository.OpenDatabasePools(config.Config{
		DatabaseDriver: "sqlite",
		SQLitePath:     ":memory:",
	})
	if err != nil {
		t.Fatalf("OpenDatabasePools error: %v", err)
	}
	if writer == nil || reader == nil {
		t.Fatal("expected non-nil db handles")
	}
}

func TestRepository_PostgresLiveRoundTrip(t *testing.T) {
	if os.Getenv("DB_DRIVER") != "postgres" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("skipping live postgres test: DB_DRIVER=postgres and DATABASE_URL not set")
	}

	cfg := config.Config{
		DatabaseDriver: "postgres",
		DatabaseDSN:    os.Getenv("DATABASE_URL"),
	}
	db, err := repository.OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("OpenDatabase postgres error: %v", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	testKey := fmt.Sprintf("postgres_test_%d", time.Now().UnixNano())
	testVal := "active"
	setting := entities.AppSetting{
		SettingKey: testKey,
		Value:      &testVal,
		ValueType:  entities.AppSettingValueTypeBool,
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatalf("failed to insert row into postgres: %v", err)
	}
	defer db.Delete(&entities.AppSetting{}, "setting_key = ?", testKey)

	var found entities.AppSetting
	if err := db.First(&found, "setting_key = ?", testKey).Error; err != nil {
		t.Fatalf("failed to query row from postgres: %v", err)
	}
	if found.Value == nil || *found.Value != testVal {
		t.Fatalf("expected value %q, got %+v", testVal, found.Value)
	}
}
