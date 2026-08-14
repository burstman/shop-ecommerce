package db

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/anthdm/superkit/db"
	"github.com/anthdm/superkit/kit"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// By default this is a pre-configured Gorm DB instance.
// Change this type based on the database package of your likings.
var dbInstance *gorm.DB

// GetConfig returns a config value by key from the shared config table.
func GetConfig(key string) (string, error) {
	var value string
	err := dbInstance.Raw("SELECT COALESCE(value, '') FROM config WHERE key = ?", key).Scan(&value).Error
	if err != nil {
		return "", fmt.Errorf("failed to get config %q: %w", key, err)
	}
	return value, nil
}

// Get returns the instantiated DB instance.
func Get() *gorm.DB {
	return dbInstance
}

// Connect initializes the database connection.
// It is called explicitly in main.go to ensure proper error handling and logging
// during the application startup sequence.
func Connect() error {
	// Load .env here to ensure variables are available before init logic runs
	if err := godotenv.Load(); err != nil {
		// Silent in production, only log in development
		if kit.IsDevelopment() {
			log.Println("Note: .env file not found, using system environment variables")
		}
	}

	// Create a default *sql.DB exposed by the superkit/db package
	// based on the given configuration.
	dbPort := os.Getenv("DB_PORT")
	config := db.Config{
		Driver:   os.Getenv("DB_DRIVER"),
		Name:     os.Getenv("DB_NAME"),
		Password: os.Getenv("DB_PASSWORD"),
		User:     os.Getenv("DB_USER"),
		Host:     os.Getenv("DB_HOST"),
	}

	// Defensively trim literal quotes and spaces that might be passed from the Makefile or .env
	config.Driver = strings.TrimSpace(strings.Trim(config.Driver, "\""))
	config.Name = strings.TrimSpace(strings.Trim(config.Name, "\""))
	config.Password = strings.TrimSpace(strings.Trim(config.Password, "\""))
	config.User = strings.TrimSpace(strings.Trim(config.User, "\""))
	config.Host = strings.TrimSpace(strings.Trim(config.Host, "\""))
	trimmedPort := strings.TrimSpace(strings.Trim(dbPort, "\""))

	var dbinst *sql.DB
	var err error

	if config.Driver == "postgres" {
		// Prioritize a full DATABASE_URL (trimmed of quotes and spaces)
		dsn := strings.TrimSpace(strings.Trim(os.Getenv("DATABASE_URL"), "\""))
		if dsn == "" {
			dsn = config.Name
		}

		// If dsn is still just a name and not a URL, construct it from individual parts
		if dsn != "" && !strings.Contains(dsn, "host=") &&
			!strings.HasPrefix(dsn, "postgres://") &&
			!strings.HasPrefix(dsn, "postgresql://") {

			sslMode := "require"
			if config.Host == "localhost" || config.Host == "127.0.0.1" {
				sslMode = "disable"
			}
			dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
				config.Host, trimmedPort, config.User, config.Password, config.Name, sslMode)
		}

		if dsn == "" {
			return fmt.Errorf("missing DB config: set DATABASE_URL or DB_NAME")
		}

		// Log sanitized connection attempt for debugging
		slog.Info("initializing postgres connection", "using_url", strings.Contains(dsn, "://"))
		dbinst, err = sql.Open("postgres", dsn)
	} else {
		dbinst, err = db.NewSQL(config)
	}

	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure a custom logger to suppress slow SQL warnings and "record not found" logs.
	// Remote databases like Neon often exceed the default 200ms threshold due to network latency.
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,  // Only log if query takes > 1s
			LogLevel:                  logger.Error, // Set log level to Error to hide warnings/info
			IgnoreRecordNotFoundError: true,         // Don't log "record not found" as an error
			Colorful:                  true,
		},
	)

	// Based on the driver create the corresponding DB instance.
	var errGorm error
	switch config.Driver {
	case db.DriverSqlite3:
		dbInstance, errGorm = gorm.Open(sqlite.New(sqlite.Config{
			Conn: dbinst,
		}), &gorm.Config{
			Logger: newLogger,
		})
		if errGorm == nil {
			// Optimize SQLite performance
			dbInstance.Exec("PRAGMA journal_mode=WAL;")   // Faster writes and better concurrency
			dbInstance.Exec("PRAGMA synchronous=NORMAL;") // Balance between speed and safety
			dbInstance.Exec("PRAGMA busy_timeout=5000;")  // Wait up to 5s if DB is locked
			dbInstance.Exec("PRAGMA foreign_keys=ON;")    // Ensure cascading deletes work
		}
	case "postgres":
		dbInstance, errGorm = gorm.Open(postgres.New(postgres.Config{
			Conn: dbinst,
		}), &gorm.Config{
			Logger:      newLogger,
			PrepareStmt: false,
		})
	case db.DriverMysql:
		// ...
	default:
		return fmt.Errorf("invalid database driver: %s", config.Driver)
	}

	if errGorm != nil {
		return fmt.Errorf("failed to initialize gorm: %w", errGorm)
	}

	return nil
}
