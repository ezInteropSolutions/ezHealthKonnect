// dicom/max_instance_size_test.go
// Proves ReceiverConfig.MaxInstanceSizeBytes actually bounds what gets
// stored/forwarded — a gap named (but left unaddressed) during the original
// Phase 2 build: "beyond a 128 MiB hard PDU-length ceiling... no
// configurable cap was found on one C-STORE's total reassembled dataset
// size." Found and closed during a 360 QA pass (October 2026).
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

func TestReceiver_MaxInstanceSizeBytes_RejectsOversizedInstance(t *testing.T) {
	const limitBytes = 1 * 1024 * 1024  // 1 MiB limit
	const pixelDataSize = 2 * 1024 * 1024 // 2 MiB payload — exceeds the limit

	pixelData := make([]byte, pixelDataSize)
	rand.Read(pixelData)

	port := findFreePort(t)
	stored := make(chan *StoredInstance, 1)
	type rejection struct{ callingAE, reason string }
	rejections := make(chan rejection, 1)

	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		MaxInstanceSizeBytes: limitBytes,
		OnInstanceStored: func(_ context.Context, inst *StoredInstance) uint16 {
			stored <- inst
			return network.StatusSuccess
		},
		OnRejected: func(_ context.Context, callingAE, reason string) {
			rejections <- rejection{callingAE, reason}
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.111222")
	if err := ds.AddByKeyword("PixelData", dataelem.OW, pixelData); err != nil {
		t.Fatalf("AddByKeyword PixelData: %v", err)
	}

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "OVERSIZED_SENDER", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := scu.Associate(ctx, nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	// go-dicom's Store() surfaces a non-success DIMSE status as an error —
	// this is the expected, correct outcome for an oversized instance.
	if err := scu.Store(ctx, ds); err == nil {
		t.Fatal("expected Store to fail for an instance exceeding MaxInstanceSizeBytes, got nil error")
	}

	select {
	case <-stored:
		t.Fatal("OnInstanceStored must not fire for a rejected, oversized instance")
	case rej := <-rejections:
		if rej.callingAE != "OVERSIZED_SENDER" {
			t.Errorf("OnRejected callingAE = %q, want %q", rej.callingAE, "OVERSIZED_SENDER")
		}
		if rej.reason == "" {
			t.Error("OnRejected reason must not be empty")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnRejected")
	}
}

func TestReceiver_MaxInstanceSizeBytes_AllowsInstanceUnderLimit(t *testing.T) {
	const limitBytes = 5 * 1024 * 1024    // 5 MiB limit
	const pixelDataSize = 1 * 1024 * 1024 // 1 MiB payload — under the limit

	pixelData := make([]byte, pixelDataSize)
	rand.Read(pixelData)

	port := findFreePort(t)
	stored := make(chan *StoredInstance, 1)

	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		MaxInstanceSizeBytes: limitBytes,
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

	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.333444")
	if err := ds.AddByKeyword("PixelData", dataelem.OW, pixelData); err != nil {
		t.Fatalf("AddByKeyword PixelData: %v", err)
	}

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "NORMAL_SENDER", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := scu.Associate(ctx, nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	if err := scu.Store(ctx, ds); err != nil {
		t.Fatalf("expected Store to succeed for an instance under MaxInstanceSizeBytes, got: %v", err)
	}

	select {
	case <-stored:
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnInstanceStored for an under-limit instance")
	}
}

func TestReceiver_MaxInstanceSizeBytes_ZeroMeansNoAppLevelBound(t *testing.T) {
	const pixelDataSize = 3 * 1024 * 1024 // arbitrary, well within the library's own hard ceiling
	pixelData := make([]byte, pixelDataSize)
	rand.Read(pixelData)

	port := findFreePort(t)
	stored := make(chan *StoredInstance, 1)

	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port, // MaxInstanceSizeBytes deliberately left at its zero value
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

	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.555666")
	if err := ds.AddByKeyword("PixelData", dataelem.OW, pixelData); err != nil {
		t.Fatalf("AddByKeyword PixelData: %v", err)
	}

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "NORMAL_SENDER", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := scu.Associate(ctx, nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	if err := scu.Store(ctx, ds); err != nil {
		t.Fatalf("expected Store to succeed when MaxInstanceSizeBytes is 0 (no app-level bound), got: %v", err)
	}

	select {
	case <-stored:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnInstanceStored with MaxInstanceSizeBytes unset")
	}
}
