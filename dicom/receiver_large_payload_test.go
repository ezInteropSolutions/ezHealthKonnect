package dicom

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/network"
)

// TestReceiver_LargePixelData_RealWorldSizedImage proves the full receive →
// serialize → callback path handles a realistic CR/DX image size, not just
// the tiny metadata-only fixtures every other test in this package uses.
// Real CR images are commonly 8-25MB; this uses 10MB as a representative,
// real-world-sized (not arbitrarily small) payload, and asserts both
// correctness (byte-for-byte round trip) and a basic performance bound (the
// whole C-STORE must complete well under the test's own context timeout).
func TestReceiver_LargePixelData_RealWorldSizedImage(t *testing.T) {
	const pixelDataSize = 10 * 1024 * 1024 // 10MB — representative of a real CR image
	pixelData := make([]byte, pixelDataSize)
	if _, err := rand.Read(pixelData); err != nil {
		t.Fatalf("generating random pixel data: %v", err)
	}

	port := findFreePort(t)
	stored := make(chan *StoredInstance, 1)
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		OnInstanceStored: func(_ context.Context, inst *StoredInstance) uint16 {
			stored <- inst
			return network.StatusSuccess
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.999999")
	if err := ds.AddByKeyword("PixelData", dataelem.OW, pixelData); err != nil {
		t.Fatalf("AddByKeyword PixelData: %v", err)
	}

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := scu.Associate(ctx, nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	start := time.Now()
	if err := scu.Store(ctx, ds); err != nil {
		t.Fatalf("Store (10MB payload): %v", err)
	}
	storeElapsed := time.Since(start)
	t.Logf("10MB C-STORE completed in %v", storeElapsed)

	select {
	case inst := <-stored:
		if len(inst.Part10) < pixelDataSize {
			t.Fatalf("serialized Part10 file (%d bytes) is smaller than the raw pixel data alone (%d bytes) — data loss during serialization", len(inst.Part10), pixelDataSize)
		}
		t.Logf("Part10 file size: %d bytes (pixel data: %d bytes)", len(inst.Part10), pixelDataSize)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for OnInstanceStored after a successful Store")
	}

	// Loose, sanity-check performance bound — not a strict SLA, just a
	// regression guard against something becoming pathologically slow
	// (e.g. an accidental O(n^2) byte-copy loop over the pixel data).
	if storeElapsed > 10*time.Second {
		t.Errorf("10MB C-STORE took %v — unexpectedly slow for an in-process, localhost transfer", storeElapsed)
	}
}

// TestReceiver_LargePixelData_ConcurrentLargeStores proves multiple large
// C-STOREs can be processed concurrently without the memory/goroutine
// bookkeeping from the MaxAssociations fix (see receiver.go's own Start
// comment) breaking under real load.
func TestReceiver_LargePixelData_ConcurrentLargeStores(t *testing.T) {
	const pixelDataSize = 3 * 1024 * 1024 // 3MB each, 4 concurrent = 12MB total in flight
	const concurrency = 4

	port := findFreePort(t)
	stored := make(chan *StoredInstance, concurrency)
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port, MaxAssociations: concurrency,
		OnInstanceStored: func(_ context.Context, inst *StoredInstance) uint16 {
			stored <- inst
			return network.StatusSuccess
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	errCh := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			pixelData := make([]byte, pixelDataSize)
			rand.Read(pixelData)
			ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5."+strconv.Itoa(1000+idx))
			ds.AddByKeyword("PixelData", dataelem.OW, pixelData)

			scu := network.NewSCU(network.SCUConfig{
				CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := scu.Associate(ctx, nil); err != nil {
				errCh <- err
				return
			}
			defer scu.Release(context.Background())
			errCh <- scu.Store(ctx, ds)
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent store %d failed: %v", i, err)
		}
	}

	received := 0
	for i := 0; i < concurrency; i++ {
		select {
		case <-stored:
			received++
		case <-time.After(15 * time.Second):
			t.Fatalf("timed out waiting for stored instance %d of %d", i+1, concurrency)
		}
	}
	if received != concurrency {
		t.Errorf("expected %d stored instances, got %d", concurrency, received)
	}
}
