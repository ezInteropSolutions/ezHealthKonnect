package dicom

import (
	"bytes"
	"io"
	"testing"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
	"github.com/amrshadid/go-dicom/filebase"
	"github.com/amrshadid/go-dicom/filereader"
)

func TestMemWriteSeeker_SequentialWrite(t *testing.T) {
	w := &memWriteSeeker{}
	n, err := w.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if string(w.buf) != "hello" {
		t.Errorf("buf = %q, want %q", w.buf, "hello")
	}
}

// TestMemWriteSeeker_SeekBackAndOverwrite exercises the exact capability
// this type exists for: a DICOM file writer backpatches a group-length
// field after writing the group's contents — write, seek back, overwrite a
// few bytes in place, seek forward again. A plain bytes.Buffer cannot do
// this at all (write-only).
func TestMemWriteSeeker_SeekBackAndOverwrite(t *testing.T) {
	w := &memWriteSeeker{}
	w.Write([]byte("AAAABBBBCCCC"))

	pos, err := w.Seek(4, io.SeekStart)
	if err != nil || pos != 4 {
		t.Fatalf("Seek(4, SeekStart): pos=%d err=%v", pos, err)
	}
	w.Write([]byte("XXXX")) // overwrite the "BBBB" in place, not append

	if string(w.buf) != "AAAAXXXXCCCC" {
		t.Errorf("buf = %q, want %q", w.buf, "AAAAXXXXCCCC")
	}

	// Seek back to the end and append further — proves the seeker tracks
	// position correctly across both overwrite and append.
	end, err := w.Seek(0, io.SeekEnd)
	if err != nil || end != 12 {
		t.Fatalf("Seek(0, SeekEnd): pos=%d err=%v", end, err)
	}
	w.Write([]byte("DDDD"))
	if string(w.buf) != "AAAAXXXXCCCCDDDD" {
		t.Errorf("buf = %q, want %q", w.buf, "AAAAXXXXCCCCDDDD")
	}
}

func TestMemWriteSeeker_SeekCurrent(t *testing.T) {
	w := &memWriteSeeker{}
	w.Write([]byte("0123456789"))
	w.Seek(0, io.SeekStart)
	pos, err := w.Seek(3, io.SeekCurrent)
	if err != nil || pos != 3 {
		t.Fatalf("Seek(3, SeekCurrent) from 0: pos=%d err=%v", pos, err)
	}
	w.Write([]byte("X"))
	if string(w.buf) != "012X456789" {
		t.Errorf("buf = %q, want %q", w.buf, "012X456789")
	}
}

func TestMemWriteSeeker_NegativeSeekRejected(t *testing.T) {
	w := &memWriteSeeker{}
	w.Write([]byte("hello"))
	if _, err := w.Seek(-1, io.SeekStart); err == nil {
		t.Error("expected a negative absolute seek position to be rejected")
	}
}

func TestMemWriteSeeker_WriteBeyondCurrentLengthGrowsBuffer(t *testing.T) {
	w := &memWriteSeeker{}
	w.Write([]byte("AB"))
	w.Seek(10, io.SeekStart) // seek past the current end
	w.Write([]byte("Z"))
	if len(w.buf) != 11 {
		t.Fatalf("buf length = %d, want 11", len(w.buf))
	}
	if w.buf[10] != 'Z' {
		t.Errorf("buf[10] = %q, want 'Z'", w.buf[10])
	}
}

// TestSerializeToPart10_RoundTrips builds a dataset directly (not via a
// real C-STORE), serializes it, and re-reads it with the library's own
// real reader — the Part 10 round-trip proof independent of any network
// activity, isolating this one function's own correctness.
func TestSerializeToPart10_RoundTrips(t *testing.T) {
	ds := dataset.NewDataset()
	if err := ds.AddByKeyword("SOPClassUID", dataelem.UI, []byte("1.2.840.10008.5.1.4.1.1.1")); err != nil {
		t.Fatalf("AddByKeyword SOPClassUID: %v", err)
	}
	if err := ds.AddByKeyword("SOPInstanceUID", dataelem.UI, []byte("1.2.3.4.5.42")); err != nil {
		t.Fatalf("AddByKeyword SOPInstanceUID: %v", err)
	}
	if err := ds.AddByKeyword("PatientID", dataelem.LO, []byte("PID42")); err != nil {
		t.Fatalf("AddByKeyword PatientID: %v", err)
	}

	out, err := SerializeToPart10("1.2.840.10008.5.1.4.1.1.1", "1.2.3.4.5.42", ds)
	if err != nil {
		t.Fatalf("SerializeToPart10: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("SerializeToPart10 returned empty bytes")
	}
	if string(out[128:132]) != "DICM" {
		t.Errorf("missing DICM magic at offset 128, got %q", out[128:132])
	}

	df, err := filereader.ReadDICOMFile(filebase.NewFileReader(bytes.NewReader(out)))
	if err != nil {
		t.Fatalf("re-reading serialized Part10: %v", err)
	}
	reDS := df.GetDataset()
	if got := reDS.GetStringByKeyword("SOPInstanceUID"); got != "1.2.3.4.5.42" {
		t.Errorf("re-parsed SOPInstanceUID = %q, want 1.2.3.4.5.42", got)
	}
	if got := reDS.GetStringByKeyword("PatientID"); got != "PID42" {
		t.Errorf("re-parsed PatientID = %q, want PID42", got)
	}
}
