// Package dbpool provides a process-wide registry of sql.DB connection pools,
// keyed by a SHA-256 hash of (dbType, connStr) so credentials never appear
// as plain-text map keys in heap dumps or debug output.
package dbpool

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"ezhealthkonnect/services/logger"
	"fmt"
	"sync"
	"time"
)

// PoolConfig controls sizing for a single database connection pool.
// Zero values cause the registry to use its built-in defaults.
type PoolConfig struct {
	MaxOpen        int           // maximum open connections (default 10)
	MaxIdle        int           // maximum idle connections (default 5)
	MaxLifetime    time.Duration // connection max lifetime (default 5m)
	ConnectTimeout time.Duration // initial ping timeout (default 2s)
}

func (c *PoolConfig) applyDefaults() {
	if c.MaxOpen <= 0 {
		c.MaxOpen = 10
	}
	if c.MaxIdle <= 0 {
		c.MaxIdle = 5
	}
	if c.MaxLifetime <= 0 {
		c.MaxLifetime = 5 * time.Minute
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 2 * time.Second
	}
}

// Registry manages one sql.DB pool per unique (dbType, connStr) pair.
type Registry struct {
	pools map[string]*sql.DB
	mu    sync.RWMutex
}

// global is the process-wide singleton.
var global = &Registry{pools: make(map[string]*sql.DB)}

// Get returns the global singleton registry.
func Get() *Registry { return global }

// GetOrCreate returns an existing live pool or opens a new one.
// cfg may be zero-valued; defaults are applied automatically.
func (r *Registry) GetOrCreate(dbType, connStr, driverName string, cfg PoolConfig) (*sql.DB, error) {
	cfg.applyDefaults()
	key := poolKey(dbType, connStr)

	// Fast path — pool exists and is alive.
	r.mu.RLock()
	if db, ok := r.pools[key]; ok {
		r.mu.RUnlock()
		if db.Ping() == nil {
			return db, nil
		}
		logger.Warn("db pool stale, recreating", "db_type", dbType)
	} else {
		r.mu.RUnlock()
	}

	// Slow path — create under write lock.
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check: another goroutine may have created it while we waited.
	if db, ok := r.pools[key]; ok {
		if db.Ping() == nil {
			return db, nil
		}
		db.Close()
		delete(r.pools, key)
	}

	logger.Info("db pool opening", "db_type", dbType)
	db, err := sql.Open(driverName, connStr)
	if err != nil {
		return nil, fmt.Errorf("dbpool: open %s: %w", dbType, err)
	}
	db.SetMaxOpenConns(cfg.MaxOpen)
	db.SetMaxIdleConns(cfg.MaxIdle)
	db.SetConnMaxLifetime(cfg.MaxLifetime)

	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("dbpool: ping %s: %w", dbType, err)
	}

	r.pools[key] = db
	logger.Info("db pool ready", "db_type", dbType)
	return db, nil
}

// Close closes and removes the pool for a specific (dbType, connStr) pair.
func (r *Registry) Close(dbType, connStr string) {
	key := poolKey(dbType, connStr)
	r.mu.Lock()
	defer r.mu.Unlock()
	if db, ok := r.pools[key]; ok {
		db.Close()
		delete(r.pools, key)
		logger.Info("db pool closed", "db_type", dbType)
	}
}

// CloseAll closes every pool in the registry.  Call this on application shutdown.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, db := range r.pools {
		db.Close()
		delete(r.pools, key)
	}
	logger.Info("db pool registry: all pools closed")
}

// poolKey returns a stable opaque key for a (dbType, connStr) pair.
func poolKey(dbType, connStr string) string {
	h := sha256.Sum256([]byte(dbType + "::" + connStr))
	return fmt.Sprintf("%x", h)
}
