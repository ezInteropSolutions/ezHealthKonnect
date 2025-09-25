// performance_benchmarks.go
// Performance benchmarking and load testing for the Interface-Centric Configuration Engine

package tests

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BenchmarkConfig holds benchmark configuration
type BenchmarkConfig struct {
	ConcurrentUsers    int
	MessagesPerUser    int
	TestDuration       time.Duration
	ConfigOperations   int
	WarmupDuration     time.Duration
}

// PerformanceMetrics holds performance measurement results
type PerformanceMetrics struct {
	TotalOperations      int64
	SuccessfulOperations int64
	FailedOperations     int64
	MinLatency          time.Duration
	MaxLatency          time.Duration
	AvgLatency          time.Duration
	P95Latency          time.Duration
	P99Latency          time.Duration
	Throughput          float64 // operations per second
	ErrorRate           float64 // percentage
	MemoryUsage         int64   // bytes
	CPUUsage            float64 // percentage
}

// BenchmarkMessageProcessing tests message processing performance
func BenchmarkMessageProcessing(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ts := SetupTestSuite(&testing.T{})
	defer ts.TeardownTestSuite(&testing.T{})

	// Setup test configuration
	config := createAdvancedTestConfiguration()
	err := ts.configManager.SaveConfig(config)
	require.NoError(b, err, "Failed to save benchmark configuration")

	// Warm up the system
	warmupMessage := createHL7ADTMessage()
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_, _ = ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(warmupMessage))
	}

	// Benchmark message processing
	b.ResetTimer()
	b.ReportAllocs()

	message := createHL7ADTMessage()
	messageBytes := []byte(message)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, messageBytes)
			if err != nil {
				b.Errorf("Message processing failed: %v", err)
			}
		}
	})
}

// BenchmarkConfigurationOperations tests configuration CRUD performance
func BenchmarkConfigurationOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ts := SetupTestSuite(&testing.T{})
	defer ts.TeardownTestSuite(&testing.T{})

	b.Run("SaveConfiguration", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				config := createAdvancedTestConfiguration()
				config.InterfaceID = uuid.New().String()
				err := ts.configManager.SaveConfig(config)
				if err != nil {
					b.Errorf("Configuration save failed: %v", err)
				}
			}
		})
	})

	b.Run("LoadConfiguration", func(b *testing.B) {
		// Pre-create configurations for loading
		configs := make([]*services.InterfaceConfig, 100)
		for i := 0; i < 100; i++ {
			config := createAdvancedTestConfiguration()
			config.InterfaceID = uuid.New().String()
			configs[i] = config
			err := ts.configManager.SaveConfig(config)
			require.NoError(b, err, "Failed to create benchmark configuration")
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				config := configs[i%len(configs)]
				_, err := ts.configManager.LoadConfig(config.InterfaceID)
				if err != nil {
					b.Errorf("Configuration load failed: %v", err)
				}
				i++
			}
		})
	})

	b.Run("ValidateConfiguration", func(b *testing.B) {
		config := createAdvancedTestConfiguration()

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				err := ts.configManager.ValidateConfig(config)
				if err != nil {
					b.Errorf("Configuration validation failed: %v", err)
				}
			}
		})
	})
}

// TestLoadTesting performs comprehensive load testing
func TestLoadTesting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load testing in short mode")
	}

	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	configs := []BenchmarkConfig{
		{
			ConcurrentUsers:  10,
			MessagesPerUser:  100,
			TestDuration:     30 * time.Second,
			ConfigOperations: 50,
			WarmupDuration:   5 * time.Second,
		},
		{
			ConcurrentUsers:  50,
			MessagesPerUser:  200,
			TestDuration:     60 * time.Second,
			ConfigOperations: 100,
			WarmupDuration:   10 * time.Second,
		},
		{
			ConcurrentUsers:  100,
			MessagesPerUser:  500,
			TestDuration:     120 * time.Second,
			ConfigOperations: 200,
			WarmupDuration:   15 * time.Second,
		},
	}

	for i, benchConfig := range configs {
		t.Run(fmt.Sprintf("LoadTest_%d_Users_%d", i+1, benchConfig.ConcurrentUsers), func(t *testing.T) {
			metrics := runLoadTest(t, ts, benchConfig)

			// Assert performance requirements
			assert.Less(t, metrics.ErrorRate, 1.0, "Error rate should be less than 1%")
			assert.Less(t, metrics.P95Latency, 1*time.Second, "95th percentile latency should be less than 1 second")
			assert.Greater(t, metrics.Throughput, float64(benchConfig.ConcurrentUsers)*0.5, "Throughput should be reasonable")

			// Log performance metrics
			t.Logf("Load Test Results for %d concurrent users:", benchConfig.ConcurrentUsers)
			t.Logf("  Total Operations: %d", metrics.TotalOperations)
			t.Logf("  Successful Operations: %d", metrics.SuccessfulOperations)
			t.Logf("  Error Rate: %.2f%%", metrics.ErrorRate)
			t.Logf("  Average Latency: %v", metrics.AvgLatency)
			t.Logf("  95th Percentile Latency: %v", metrics.P95Latency)
			t.Logf("  99th Percentile Latency: %v", metrics.P99Latency)
			t.Logf("  Throughput: %.2f ops/sec", metrics.Throughput)
			t.Logf("  Memory Usage: %.2f MB", float64(metrics.MemoryUsage)/(1024*1024))
		})
	}
}

// runLoadTest executes a load test with the given configuration
func runLoadTest(t *testing.T, ts *TestSuite, config BenchmarkConfig) *PerformanceMetrics {
	// Setup test interface configuration
	testConfig := createAdvancedTestConfiguration()
	err := ts.configManager.SaveConfig(testConfig)
	require.NoError(t, err, "Failed to save test configuration")

	// Warmup phase
	log.Printf("Starting warmup phase for %v", config.WarmupDuration)
	warmupCtx, warmupCancel := context.WithTimeout(context.Background(), config.WarmupDuration)
	defer warmupCancel()

	warmupMessage := createHL7ADTMessage()
	for warmupCtx.Err() == nil {
		_, _ = ts.interfaceEngine.ProcessMessage(warmupCtx, testConfig.InterfaceID, []byte(warmupMessage))
		time.Sleep(10 * time.Millisecond)
	}

	// Prepare metrics collection
	latencies := make([]time.Duration, 0, config.ConcurrentUsers*config.MessagesPerUser)
	var latenciesMutex sync.Mutex
	var successCount, errorCount int64
	var successMutex, errorMutex sync.Mutex

	// Start memory monitoring
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	startMemory := memStats.Alloc

	// Load test phase
	log.Printf("Starting load test with %d concurrent users for %v", config.ConcurrentUsers, config.TestDuration)

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)
	defer cancel()

	var wg sync.WaitGroup
	startTime := time.Now()

	// Start concurrent users
	for i := 0; i < config.ConcurrentUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			message := createHL7ADTMessage()
			messageProcessed := 0

			for ctx.Err() == nil && messageProcessed < config.MessagesPerUser {
				operationStart := time.Now()

				_, err := ts.interfaceEngine.ProcessMessage(ctx, testConfig.InterfaceID, []byte(message))

				latency := time.Since(operationStart)
				latenciesMutex.Lock()
				latencies = append(latencies, latency)
				latenciesMutex.Unlock()

				if err != nil {
					errorMutex.Lock()
					errorCount++
					errorMutex.Unlock()
				} else {
					successMutex.Lock()
					successCount++
					successMutex.Unlock()
				}

				messageProcessed++

				// Small delay to prevent overwhelming the system
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait for all users to complete
	wg.Wait()
	testDuration := time.Since(startTime)

	// Calculate final memory usage
	runtime.ReadMemStats(&memStats)
	endMemory := memStats.Alloc
	memoryUsed := endMemory - startMemory

	// Calculate performance metrics
	totalOps := successCount + errorCount
	errorRate := float64(errorCount) / float64(totalOps) * 100
	throughput := float64(totalOps) / testDuration.Seconds()

	// Calculate latency percentiles
	metrics := &PerformanceMetrics{
		TotalOperations:      totalOps,
		SuccessfulOperations: successCount,
		FailedOperations:     errorCount,
		ErrorRate:           errorRate,
		Throughput:          throughput,
		MemoryUsage:         int64(memoryUsed),
	}

	if len(latencies) > 0 {
		// Sort latencies for percentile calculation
		sortLatencies(latencies)

		metrics.MinLatency = latencies[0]
		metrics.MaxLatency = latencies[len(latencies)-1]
		metrics.AvgLatency = calculateAverageLatency(latencies)
		metrics.P95Latency = latencies[int(float64(len(latencies))*0.95)]
		metrics.P99Latency = latencies[int(float64(len(latencies))*0.99)]
	}

	return metrics
}

// TestMemoryLeaks performs memory leak detection
func TestMemoryLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak testing in short mode")
	}

	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	// Setup test configuration
	config := createAdvancedTestConfiguration()
	err := ts.configManager.SaveConfig(config)
	require.NoError(t, err, "Failed to save test configuration")

	// Baseline memory measurement
	runtime.GC()
	runtime.GC() // Double GC to ensure cleanup
	var baselineMemStats runtime.MemStats
	runtime.ReadMemStats(&baselineMemStats)
	baselineMemory := baselineMemStats.Alloc

	t.Logf("Baseline memory usage: %.2f MB", float64(baselineMemory)/(1024*1024))

	// Process many messages to detect memory leaks
	message := createHL7ADTMessage()
	messageBytes := []byte(message)
	ctx := context.Background()

	iterations := 10000
	for i := 0; i < iterations; i++ {
		_, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, messageBytes)
		if err != nil {
			t.Errorf("Message processing failed at iteration %d: %v", i, err)
		}

		// Periodic memory check
		if i%1000 == 0 {
			runtime.GC()
			var currentMemStats runtime.MemStats
			runtime.ReadMemStats(&currentMemStats)
			currentMemory := currentMemStats.Alloc

			t.Logf("Iteration %d memory usage: %.2f MB", i, float64(currentMemory)/(1024*1024))
		}
	}

	// Final memory measurement
	runtime.GC()
	runtime.GC() // Double GC to ensure cleanup
	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	finalMemory := finalMemStats.Alloc

	t.Logf("Final memory usage: %.2f MB", float64(finalMemory)/(1024*1024))

	// Check for memory leaks (allow for some growth due to caching, etc.)
	memoryGrowth := finalMemory - baselineMemory
	maxAllowedGrowth := baselineMemory / 2 // Allow 50% growth

	assert.Less(t, memoryGrowth, maxAllowedGrowth,
		"Memory usage grew too much (%.2f MB), possible memory leak detected",
		float64(memoryGrowth)/(1024*1024))

	t.Logf("Memory growth: %.2f MB (%.1f%% of baseline)",
		float64(memoryGrowth)/(1024*1024),
		float64(memoryGrowth)/float64(baselineMemory)*100)
}

// TestConcurrentConfigurationOperations tests concurrent configuration access
func TestConcurrentConfigurationOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent testing in short mode")
	}

	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	// Create multiple configurations for testing
	configs := make([]*services.InterfaceConfig, 50)
	for i := 0; i < 50; i++ {
		config := createAdvancedTestConfiguration()
		config.InterfaceID = uuid.New().String()
		config.Name = fmt.Sprintf("Concurrent Test Config %d", i)
		configs[i] = config

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save concurrent test configuration")
	}

	// Test concurrent read operations
	t.Run("ConcurrentReads", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 100)

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				config := configs[index%len(configs)]
				_, err := ts.configManager.LoadConfig(config.InterfaceID)
				if err != nil {
					errors <- fmt.Errorf("concurrent read %d failed: %w", index, err)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		errorCount := 0
		for err := range errors {
			t.Errorf("Concurrent read error: %v", err)
			errorCount++
		}

		assert.Equal(t, 0, errorCount, "No errors should occur during concurrent reads")
	})

	// Test concurrent write operations
	t.Run("ConcurrentWrites", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 50)

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				config := configs[index]
				config.Description = fmt.Sprintf("Updated by concurrent write %d", index)
				config.Version = fmt.Sprintf("2.%d.0", index)

				err := ts.configManager.SaveConfig(config)
				if err != nil {
					errors <- fmt.Errorf("concurrent write %d failed: %w", index, err)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		errorCount := 0
		for err := range errors {
			t.Errorf("Concurrent write error: %v", err)
			errorCount++
		}

		assert.Equal(t, 0, errorCount, "No errors should occur during concurrent writes")
	})

	// Test mixed read/write operations
	t.Run("MixedOperations", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 200)

		// Start readers
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				config := configs[index%len(configs)]
				_, err := ts.configManager.LoadConfig(config.InterfaceID)
				if err != nil {
					errors <- fmt.Errorf("mixed read %d failed: %w", index, err)
				}
			}(i)
		}

		// Start writers
		for i := 0; i < 25; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				config := configs[index]
				config.Description = fmt.Sprintf("Updated by mixed write %d", index)

				err := ts.configManager.SaveConfig(config)
				if err != nil {
					errors <- fmt.Errorf("mixed write %d failed: %w", index, err)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		errorCount := 0
		for err := range errors {
			t.Errorf("Mixed operation error: %v", err)
			errorCount++
		}

		assert.Less(t, errorCount, 5, "Very few errors should occur during mixed operations")
	})
}

// Helper functions for performance calculations

func sortLatencies(latencies []time.Duration) {
	// Simple bubble sort for small datasets (good enough for testing)
	n := len(latencies)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if latencies[j] > latencies[j+1] {
				latencies[j], latencies[j+1] = latencies[j+1], latencies[j]
			}
		}
	}
}

func calculateAverageLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	var total time.Duration
	for _, latency := range latencies {
		total += latency
	}

	return total / time.Duration(len(latencies))
}

// TestResourceUsage monitors system resource usage during operation
func TestResourceUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource usage testing in short mode")
	}

	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	// Setup test configuration
	config := createAdvancedTestConfiguration()
	err := ts.configManager.SaveConfig(config)
	require.NoError(t, err, "Failed to save test configuration")

	// Monitor resource usage during normal operation
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start resource monitoring
	var maxMemory uint64
	var memoryMeasurements []uint64

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)

				currentMemory := memStats.Alloc
				memoryMeasurements = append(memoryMeasurements, currentMemory)

				if currentMemory > maxMemory {
					maxMemory = currentMemory
				}
			}
		}
	}()

	// Generate load while monitoring resources
	message := createHL7ADTMessage()
	messageBytes := []byte(message)

	operationCount := 0
	for ctx.Err() == nil {
		_, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, messageBytes)
		if err != nil {
			t.Logf("Message processing error: %v", err)
		} else {
			operationCount++
		}

		// Small delay to make it realistic
		time.Sleep(10 * time.Millisecond)
	}

	// Analyze resource usage
	t.Logf("Resource Usage Analysis:")
	t.Logf("  Operations completed: %d", operationCount)
	t.Logf("  Max memory usage: %.2f MB", float64(maxMemory)/(1024*1024))
	t.Logf("  Memory measurements: %d", len(memoryMeasurements))

	// Calculate average memory usage
	if len(memoryMeasurements) > 0 {
		var totalMemory uint64
		for _, mem := range memoryMeasurements {
			totalMemory += mem
		}
		avgMemory := totalMemory / uint64(len(memoryMeasurements))
		t.Logf("  Average memory usage: %.2f MB", float64(avgMemory)/(1024*1024))

		// Assert memory usage is reasonable (adjust based on your requirements)
		assert.Less(t, maxMemory, uint64(512*1024*1024), "Max memory usage should be less than 512MB")
	}

	// Check that we processed a reasonable number of operations
	assert.Greater(t, operationCount, 100, "Should process at least 100 operations in 60 seconds")
}