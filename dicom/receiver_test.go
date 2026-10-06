package dicom

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
	"github.com/amrshadid/go-dicom/filebase"
	"github.com/amrshadid/go-dicom/filereader"
	"github.com/amrshadid/go-dicom/network"
)

// findFreePort asks the OS for a free TCP port by briefly binding to :0.
func findFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// buildTestDataset builds a minimal, real, valid dataset for a test
// C-STORE — just enough fields for ExtractMetadata/SerializeToPart10 to
// exercise real code, not a synthetic struct literal.
func buildTestDataset(t *testing.T, sopClassUID, sopInstanceUID string) *dataset.Dataset {
	t.Helper()
	ds := dataset.NewDataset()
	must := func(err error) {
		if err != nil {
			t.Fatalf("buildTestDataset: %v", err)
		}
	}
	must(ds.AddByKeyword("SOPClassUID", dataelem.UI, []byte(sopClassUID)))
	must(ds.AddByKeyword("SOPInstanceUID", dataelem.UI, []byte(sopInstanceUID)))
	must(ds.AddByKeyword("PatientID", dataelem.LO, []byte("TESTPAT001")))
	must(ds.AddByKeyword("PatientName", dataelem.PN, []byte("Test^Patient")))
	must(ds.AddByKeyword("StudyInstanceUID", dataelem.UI, []byte("1.2.3.4.5")))
	must(ds.AddByKeyword("SeriesInstanceUID", dataelem.UI, []byte("1.2.3.4.5.1")))
	must(ds.AddByKeyword("Modality", dataelem.CS, []byte("CR")))
	return ds
}

// startTestReceiver starts a real Receiver on an ephemeral port and returns
// it along with a channel the test can read OnInstanceStored calls from.
func startTestReceiver(t *testing.T, cfg ReceiverConfig) (*Receiver, <-chan *StoredInstance) {
	t.Helper()
	stored := make(chan *StoredInstance, 10)
	cfg.OnInstanceStored = func(_ context.Context, inst *StoredInstance) uint16 {
		stored <- inst
		return network.StatusSuccess
	}
	r := NewReceiver(cfg)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Receiver.Start: %v", err)
	}
	t.Cleanup(r.Stop)
	return r, stored
}

func TestReceiver_CEcho_Success(t *testing.T) {
	port := findFreePort(t)
	startTestReceiver(t, ReceiverConfig{AETitle: "TESTSCP", Port: port})
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	if err := scu.Echo(context.Background()); err != nil {
		t.Fatalf("Echo: %v", err)
	}
}

func TestReceiver_CStore_Success_RoundTripsRealPart10File(t *testing.T) {
	port := findFreePort(t)
	_, stored := startTestReceiver(t, ReceiverConfig{AETitle: "TESTSCP", Port: port})
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.9999")
	if err := scu.Store(context.Background(), ds); err != nil {
		t.Fatalf("Store: %v", err)
	}

	select {
	case inst := <-stored:
		if inst.SOPInstanceUID != "1.2.3.4.5.9999" {
			t.Errorf("SOPInstanceUID = %q, want 1.2.3.4.5.9999", inst.SOPInstanceUID)
		}
		if inst.SOPClassUID != network.CRImageStorageUID {
			t.Errorf("SOPClassUID = %q, want %q", inst.SOPClassUID, network.CRImageStorageUID)
		}
		if inst.CallingAE != "TESTSCU" {
			t.Errorf("CallingAE = %q, want TESTSCU", inst.CallingAE)
		}
		if inst.Metadata.PatientID != "TESTPAT001" {
			t.Errorf("Metadata.PatientID = %q, want TESTPAT001", inst.Metadata.PatientID)
		}
		if len(inst.Part10) == 0 {
			t.Fatal("Part10 bytes are empty")
		}
		// Re-parse the serialized Part 10 bytes with the library's OWN real
		// reader — proving the bytes are not just non-empty but a genuinely
		// valid, re-decodable DICOM file.
		df, err := filereader.ReadDICOMFile(filebase.NewFileReader(bytes.NewReader(inst.Part10)))
		if err != nil {
			t.Fatalf("re-reading serialized Part10: %v", err)
		}
		reDS := df.GetDataset()
		if got := reDS.GetStringByKeyword("SOPInstanceUID"); got != "1.2.3.4.5.9999" {
			t.Errorf("re-parsed SOPInstanceUID = %q, want 1.2.3.4.5.9999", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnInstanceStored")
	}
}

func TestReceiver_AllowedCallingAETitles_RejectsDisallowedCaller(t *testing.T) {
	port := findFreePort(t)
	startTestReceiver(t, ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		AllowedCallingAETitles: []string{"ONLY_THIS_ONE"},
	})
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "SOME_OTHER_AE", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	// Both C-ECHO and C-STORE must be refused — a disallowed AE shouldn't
	// even get a successful ping.
	if err := scu.Echo(context.Background()); err == nil {
		t.Error("expected Echo from a disallowed calling AE to fail, got nil error")
	}

	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.8888")
	if err := scu.Store(context.Background(), ds); err == nil {
		t.Error("expected Store from a disallowed calling AE to fail, got nil error")
	}
}

// TestReceiver_OnRejected_FiresForDisallowedCallingAE proves the real,
// previously-missing diagnosability gap found during a QA pass (October
// 2026) is actually fixed: before OnRejected existed, a rejected
// association left zero trace anywhere in this application (no log line,
// no message row — a rejected C-STORE never reaches OnInstanceStored). This
// asserts the hook fires, with the real calling AE and a non-empty reason,
// for both the C-ECHO and C-STORE refusal paths.
func TestReceiver_OnRejected_FiresForDisallowedCallingAE(t *testing.T) {
	port := findFreePort(t)
	type rejection struct{ callingAE, reason string }
	rejections := make(chan rejection, 10)

	cfg := ReceiverConfig{
		AETitle:                "TESTSCP",
		Port:                   port,
		AllowedCallingAETitles: []string{"ONLY_THIS_ONE"},
		OnRejected: func(_ context.Context, callingAE, reason string) {
			rejections <- rejection{callingAE, reason}
		},
	}
	startTestReceiver(t, cfg)
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "SOME_OTHER_AE", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	scu.Echo(context.Background())
	ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.7777")
	scu.Store(context.Background(), ds)

	got := map[string]int{}
	timeout := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case r := <-rejections:
			if r.callingAE != "SOME_OTHER_AE" {
				t.Errorf("OnRejected callingAE = %q, want %q", r.callingAE, "SOME_OTHER_AE")
			}
			if r.reason == "" {
				t.Error("OnRejected reason must not be empty")
			}
			got[r.reason]++
		case <-timeout:
			t.Fatalf("timed out waiting for 2 distinct OnRejected calls (C-ECHO + C-STORE); got %d: %v", len(got), got)
		}
	}
}

func TestReceiver_AllowedCallingAETitles_EmptyMeansAcceptAny(t *testing.T) {
	port := findFreePort(t)
	startTestReceiver(t, ReceiverConfig{AETitle: "TESTSCP", Port: port}) // no allowlist configured
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "ANY_AE_AT_ALL", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	if err := scu.Echo(context.Background()); err != nil {
		t.Errorf("expected Echo to succeed with no allowlist configured, got: %v", err)
	}
}

// TestReceiver_AbstractSyntaxWhitelist_QueryRetrieveContextRefused proves the
// whitelist's real, confirmed effect: proposing ONLY Query/Retrieve contexts
// does NOT fail Associate itself (a real go-dicom behavior, confirmed by
// reading the library's own log output during test development: an
// association with zero usable presentation contexts is still accepted —
// DICOM allows that) — but every one of those contexts is refused, so a
// subsequent operation that needs one must fail. This is the real,
// meaningful assertion, not whether Associate returns an error.
func TestReceiver_AbstractSyntaxWhitelist_QueryRetrieveContextRefused(t *testing.T) {
	port := findFreePort(t)
	startTestReceiver(t, ReceiverConfig{AETitle: "TESTSCP", Port: port})
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), network.QueryRetrievePresentationContexts()); err != nil {
		t.Fatalf("Associate: %v (expected this to succeed even with zero usable contexts)", err)
	}
	defer scu.Release(context.Background())

	queryDS := dataset.NewDataset()
	_, err := scu.FindWithSOPClass(context.Background(), network.PatientRootQueryRetrieveFind, queryDS)
	if err == nil {
		t.Error("expected C-FIND to fail — this SCP's abstract-syntax whitelist never accepted a Query/Retrieve context")
	}
}

// TestReceiver_MaxAssociations_BoundsConcurrentCStoreProcessing is a
// deterministic concurrency test: it forces two C-STORE operations to be
// in flight at the same instant (the second is only attempted once the
// first is confirmed to be blocked inside the handler) against
// MaxAssociations=1, and asserts the second is refused with
// StatusOutOfResources while the first still succeeds once unblocked.
//
// This bounds concurrent DIMSE OPERATION processing, not raw association
// count — see Start's own comment in receiver.go for why: go-dicom
// v1.6.0's StartServer does not enforce SCPConfig.MaxAssociations at all
// (confirmed by reading network/server.go — its own accept loop omits the
// semaphore ListenAndServe's accept loop has), so this codebase enforces
// the bound itself, around HandleCStore.
func TestReceiver_MaxAssociations_BoundsConcurrentCStoreProcessing(t *testing.T) {
	port := findFreePort(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port, MaxAssociations: 1,
		OnInstanceStored: func(_ context.Context, _ *StoredInstance) uint16 {
			started <- struct{}{}
			<-release
			return network.StatusSuccess
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	addr := "127.0.0.1:" + strconv.Itoa(port)
	errCh1 := make(chan error, 1)
	go func() {
		scu1 := network.NewSCU(network.SCUConfig{CallingAE: "SCU1", CalledAE: "TESTSCP", Address: addr})
		if err := scu1.Associate(context.Background(), nil); err != nil {
			errCh1 <- err
			return
		}
		defer scu1.Release(context.Background())
		ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.111")
		errCh1 <- scu1.Store(context.Background(), ds)
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the first store to enter the handler")
	}

	// The only slot is now held by the first, still-blocked store. A
	// second, concurrent Store must be refused immediately — tryAcquireSlot
	// never blocks.
	scu2 := network.NewSCU(network.SCUConfig{CallingAE: "SCU2", CalledAE: "TESTSCP", Address: addr})
	if err := scu2.Associate(context.Background(), nil); err != nil {
		t.Fatalf("second Associate: %v (association itself is not bounded, only C-STORE processing)", err)
	}
	defer scu2.Release(context.Background())
	ds2 := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.5.222")
	err2 := scu2.Store(context.Background(), ds2)
	if err2 == nil {
		t.Error("expected the second, concurrent Store to be refused with StatusOutOfResources while the first is still in flight")
	}

	close(release)
	if err1 := <-errCh1; err1 != nil {
		t.Errorf("first store should have succeeded once the slot was released, got: %v", err1)
	}
}

// TestReceiver_ConcurrentStores_NoDataRace drives several real, concurrent
// C-STORE operations (separate associations) against one Receiver — a
// regression guard for the shared dicomHandler.allowed map and the
// OnInstanceStored closure's own captured state, run under -race.
func TestReceiver_ConcurrentStores_NoDataRace(t *testing.T) {
	port := findFreePort(t)
	var mu sync.Mutex
	var count int
	cfg := ReceiverConfig{
		AETitle: "TESTSCP", Port: port, MaxAssociations: 10,
		OnInstanceStored: func(_ context.Context, _ *StoredInstance) uint16 {
			mu.Lock()
			count++
			mu.Unlock()
			return network.StatusSuccess
		},
	}
	r := NewReceiver(cfg)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	const n = 5
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			scu := network.NewSCU(network.SCUConfig{
				CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
			})
			if err := scu.Associate(context.Background(), nil); err != nil {
				t.Errorf("Associate %d: %v", idx, err)
				return
			}
			defer scu.Release(context.Background())
			ds := buildTestDataset(t, network.CRImageStorageUID, "1.2.3.4.9."+strconv.Itoa(idx))
			if err := scu.Store(context.Background(), ds); err != nil {
				t.Errorf("Store %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	mu.Lock()
	got := count
	mu.Unlock()
	if got != n {
		t.Errorf("expected %d stores, got %d", n, got)
	}
}
