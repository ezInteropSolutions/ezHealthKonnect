// Package dbpool — white-box unit tests for the DB connection pool registry.
//
// These tests use a registered fake SQL driver so no real database is required.
// All tests operate on a fresh *Registry created inline, not the global singleton,
// so they are safe to run in parallel.
package dbpool

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/services/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain initialises the structured logger before any test runs.
// Without this, logger.L is nil and pool_registry.go panics on the first log call.
func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// ─── Fake SQL driver (no real database needed) ───────────────────────────────

// fakeDriver is the minimal sql.Driver needed to satisfy database/sql.
// It opens connections that always succeed. Registered once via init().
type fakeDriver struct{}

// fakeConn satisfies driver.Conn; does NOT implement driver.Pinger, so
// sql.DB.Ping() will succeed without sending a real query.
type fakeConn struct{}

func (fakeDriver) Open(_ string) (driver.Conn, error) { return fakeConn{}, nil }
func (fakeConn) Prepare(_ string) (driver.Stmt, error) { return nil, fmt.Errorf("not implemented") }
func (fakeConn) Close() error                          { return nil }
func (fakeConn) Begin() (driver.Tx, error)             { return nil, fmt.Errorf("not implemented") }

// failDriver always returns an error from Open, exercising error-path branches.
type failDriver struct{}

func (failDriver) Open(_ string) (driver.Conn, error) { return nil, fmt.Errorf("connection refused") }

func init() {
	sql.Register("fake_dbpool", fakeDriver{})
	sql.Register("fail_dbpool", failDriver{})
}

// newTestRegistry creates a fresh, isolated registry for each test.
// Tests must call defer reg.CloseAll() to clean up opened *sql.DB handles.
func newTestRegistry() *Registry {
	return &Registry{pools: make(map[string]*sql.DB)}
}

// ─── poolKey ─────────────────────────────────────────────────────────────────

func TestPoolKeyDeterministic(t *testing.T) {
	k1 := poolKey("postgres", "host=localhost dbname=test")
	k2 := poolKey("postgres", "host=localhost dbname=test")
	assert.Equal(t, k1, k2, "same input must produce the same key")
}

func TestPoolKeyDifferentDbType(t *testing.T) {
	k1 := poolKey("postgres", "host=a")
	k2 := poolKey("mysql", "host=a")
	assert.NotEqual(t, k1, k2, "different dbType must produce different keys")
}

func TestPoolKeyDifferentConnStr(t *testing.T) {
	k1 := poolKey("postgres", "host=a")
	k2 := poolKey("postgres", "host=b")
	assert.NotEqual(t, k1, k2, "different connStr must produce different keys")
}

func TestPoolKeyIsOpaqueHash(t *testing.T) {
	connStr := "host=db user=admin password=topsecret dbname=prod"
	key := poolKey("postgres", connStr)
	assert.NotContains(t, key, "topsecret", "key must not contain plaintext password")
	assert.NotContains(t, key, "admin", "key must not contain plaintext username")
	assert.Len(t, key, 64, "SHA-256 hex digest must be exactly 64 characters")
}

// ─── PoolConfig.applyDefaults ─────────────────────────────────────────────────

func TestApplyDefaultsZeroValues(t *testing.T) {
	cfg := PoolConfig{}
	cfg.applyDefaults()

	assert.Equal(t, 10, cfg.MaxOpen, "zero MaxOpen should default to 10")
	assert.Equal(t, 5, cfg.MaxIdle, "zero MaxIdle should default to 5")
	assert.Equal(t, 5*time.Minute, cfg.MaxLifetime, "zero MaxLifetime should default to 5m")
	assert.Equal(t, 2*time.Second, cfg.ConnectTimeout, "zero ConnectTimeout should default to 2s")
}

func TestApplyDefaultsPreservesExplicitValues(t *testing.T) {
	cfg := PoolConfig{
		MaxOpen:        20,
		MaxIdle:        8,
		MaxLifetime:    10 * time.Minute,
		ConnectTimeout: 5 * time.Second,
	}
	cfg.applyDefaults()

	assert.Equal(t, 20, cfg.MaxOpen)
	assert.Equal(t, 8, cfg.MaxIdle)
	assert.Equal(t, 10*time.Minute, cfg.MaxLifetime)
	assert.Equal(t, 5*time.Second, cfg.ConnectTimeout)
}

func TestApplyDefaultsNegativeValues(t *testing.T) {
	cfg := PoolConfig{MaxOpen: -1, MaxIdle: -5}
	cfg.applyDefaults()

	assert.Equal(t, 10, cfg.MaxOpen, "negative MaxOpen should be replaced with default")
	assert.Equal(t, 5, cfg.MaxIdle, "negative MaxIdle should be replaced with default")
}

// ─── GetOrCreate ─────────────────────────────────────────────────────────────

func TestGetOrCreateSuccess(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	db, err := reg.GetOrCreate("fake_db", "conn=1", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)
	require.NotNil(t, db)
}

func TestGetOrCreateReturnsSamePoolOnRepeatCall(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	db1, err := reg.GetOrCreate("fake_db", "conn=1", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	db2, err := reg.GetOrCreate("fake_db", "conn=1", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	assert.Same(t, db1, db2, "repeated call with same args must return the same *sql.DB pointer")
}

func TestGetOrCreateDifferentConnStringsGetDifferentPools(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	db1, err := reg.GetOrCreate("fake_db", "conn=1", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	db2, err := reg.GetOrCreate("fake_db", "conn=2", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	assert.NotSame(t, db1, db2, "different conn strings must produce different *sql.DB pools")
}

func TestGetOrCreateDifferentDbTypesGetDifferentPools(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	db1, err := reg.GetOrCreate("type_a", "conn=1", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	db2, err := reg.GetOrCreate("type_b", "conn=1", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	assert.NotSame(t, db1, db2)
}

func TestGetOrCreateInvalidDriverReturnsError(t *testing.T) {
	reg := newTestRegistry()

	_, err := reg.GetOrCreate("unknown_db", "conn=x", "no_such_driver_xyz", PoolConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dbpool: open", "error message should identify the dbpool source")
}

func TestGetOrCreateFailDriverReturnsError(t *testing.T) {
	reg := newTestRegistry()

	// fail_dbpool.Open() always returns an error, so PingContext will fail.
	_, err := reg.GetOrCreate("fail_db", "conn=fail", "fail_dbpool", PoolConfig{
		ConnectTimeout: 100 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dbpool: ping", "ping error should be wrapped with dbpool prefix")
}

// ─── Close ───────────────────────────────────────────────────────────────────

func TestCloseRemovesPool(t *testing.T) {
	reg := newTestRegistry()

	_, err := reg.GetOrCreate("fake_db", "conn=close_me", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	reg.Close("fake_db", "conn=close_me")

	key := poolKey("fake_db", "conn=close_me")
	reg.mu.RLock()
	_, exists := reg.pools[key]
	reg.mu.RUnlock()

	assert.False(t, exists, "pool must be removed from the map after Close")
}

func TestCloseNonExistentIsNoOp(t *testing.T) {
	reg := newTestRegistry()
	assert.NotPanics(t, func() {
		reg.Close("does_not_exist", "conn=missing")
	})
}

func TestCloseDoesNotAffectOtherPools(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	_, err := reg.GetOrCreate("fake_db", "conn=keep", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)
	_, err = reg.GetOrCreate("fake_db", "conn=remove", "fake_dbpool", PoolConfig{})
	require.NoError(t, err)

	reg.Close("fake_db", "conn=remove")

	// "conn=keep" must still be present
	keepKey := poolKey("fake_db", "conn=keep")
	reg.mu.RLock()
	_, keepExists := reg.pools[keepKey]
	reg.mu.RUnlock()

	assert.True(t, keepExists, "unrelated pool must remain after targeted Close")
}

// ─── CloseAll ────────────────────────────────────────────────────────────────

func TestCloseAllEmptiesRegistry(t *testing.T) {
	reg := newTestRegistry()

	for i := 0; i < 3; i++ {
		_, err := reg.GetOrCreate("fake_db", fmt.Sprintf("conn=%d", i), "fake_dbpool", PoolConfig{})
		require.NoError(t, err)
	}

	reg.CloseAll()

	reg.mu.RLock()
	count := len(reg.pools)
	reg.mu.RUnlock()

	assert.Equal(t, 0, count, "all pools must be removed after CloseAll")
}

func TestCloseAllOnEmptyRegistryIsNoOp(t *testing.T) {
	reg := newTestRegistry()
	assert.NotPanics(t, func() { reg.CloseAll() })
}

// ─── Concurrency ─────────────────────────────────────────────────────────────

func TestGetOrCreateConcurrentSameKey(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	const goroutines = 50
	results := make([]*sql.DB, goroutines)
	errors := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			db, err := reg.GetOrCreate("fake_db", "conn=shared", "fake_dbpool", PoolConfig{})
			results[idx] = db
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	// All goroutines must succeed with the same *sql.DB pointer.
	first := results[0]
	require.NotNil(t, first)
	for i, db := range results {
		require.NoError(t, errors[i], "goroutine %d returned an error", i)
		assert.Same(t, first, db, "goroutine %d got a different pool pointer", i)
	}
}

func TestGetOrCreateConcurrentDifferentKeys(t *testing.T) {
	reg := newTestRegistry()
	defer reg.CloseAll()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := reg.GetOrCreate("fake_db", fmt.Sprintf("conn=g%d", idx), "fake_dbpool", PoolConfig{})
			assert.NoError(t, err, "goroutine %d should not error", idx)
		}(i)
	}
	wg.Wait()

	reg.mu.RLock()
	count := len(reg.pools)
	reg.mu.RUnlock()

	assert.Equal(t, goroutines, count, "each unique conn string must produce its own pool")
}

// ─── Get (singleton) ─────────────────────────────────────────────────────────

func TestGetReturnsSamePointer(t *testing.T) {
	r1 := Get()
	r2 := Get()
	assert.Same(t, r1, r2, "Get() must always return the same global singleton")
}
