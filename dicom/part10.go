// dicom/part10.go
// In-memory DICOM Part 10 file serialization. Needed because
// filebase.NewFileWriter requires an io.WriteSeeker (a DICOM file writer
// backpatches group-length fields after writing their contents, which a
// write-only io.Writer like bytes.Buffer cannot support) — and this app is
// middleware, not a PACS, so a received image is never staged on local disk
// as the system of record. memWriteSeeker is the one small primitive that
// closes that gap.
package dicom

import (
	"errors"
	"io"

	"github.com/amrshadid/go-dicom/dataset"
	"github.com/amrshadid/go-dicom/filebase"
	"github.com/amrshadid/go-dicom/filewriter"
	"github.com/amrshadid/go-dicom/network"
)

// memWriteSeeker is a minimal io.WriteSeeker over an in-memory byte slice.
type memWriteSeeker struct {
	buf []byte
	pos int
}

func (w *memWriteSeeker) Write(p []byte) (int, error) {
	end := w.pos + len(p)
	if end > len(w.buf) {
		grown := make([]byte, end)
		copy(grown, w.buf)
		w.buf = grown
	}
	copy(w.buf[w.pos:end], p)
	w.pos = end
	return len(p), nil
}

func (w *memWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = int64(w.pos) + offset
	case io.SeekEnd:
		newPos = int64(len(w.buf)) + offset
	default:
		return 0, errors.New("dicom: memWriteSeeker: invalid whence")
	}
	if newPos < 0 {
		return 0, errors.New("dicom: memWriteSeeker: negative seek position")
	}
	w.pos = int(newPos)
	return newPos, nil
}

// SerializeToPart10 writes a received dataset out as a real DICOM Part 10
// file (preamble + "DICM" magic + file meta information + data set), fully
// in memory. Mirrors go-dicom's own cli/storescp.go writeDICOMFile function
// (the library's own real, tested reference implementation for exactly this
// operation) line-for-line, substituting the in-memory writer above for a
// real *os.File.
func SerializeToPart10(sopClassUID, sopInstanceUID string, ds *dataset.Dataset) ([]byte, error) {
	mws := &memWriteSeeker{}

	w := filewriter.NewDICOMFileWriter(filebase.NewFileWriter(mws))
	w.SetFileMetaInfo(&filewriter.FileMetaInfo{
		MediaStorageSOPClassUID:    sopClassUID,
		MediaStorageSOPInstanceUID: sopInstanceUID,
		TransferSyntaxUID:          filewriter.StorageTransferSyntax(ds),
		ImplementationClassUID:     network.DefaultImplementationClassUID,
		ImplementationVersionName:  network.DefaultImplementationVersionName,
	})

	for _, elem := range filewriter.ElementsFromDataset(ds) {
		if err := w.AddDataElement(elem); err != nil {
			return nil, err
		}
	}

	if err := w.Write(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return mws.buf, nil
}
