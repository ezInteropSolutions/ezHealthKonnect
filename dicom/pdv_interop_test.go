// dicom/pdv_interop_test.go
// Proves the local go-dicom fork's fix (third_party/go-dicom-fork/
// PATCHES.md) for a real, confirmed interop bug: a single PDataTF PDU
// carrying a Command message's final PDV immediately followed by a Data
// message's own PDV — the exact shape a real dcm4che/storescu C-STORE
// sends, and the exact shape that hung the unpatched library indefinitely
// (confirmed by independent testing against that real, third-party DICOM
// toolkit, not against this library's own SCU client).
//
// This test hand-crafts that PDU directly at the network.Transport/
// Association layer — below dicom.Receiver — so it exercises exactly the
// mechanism that was broken, independent of anything this package's own
// higher-level code does.
package dicom

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/amrshadid/go-dicom/network"
)

func TestReceivePData_CommandAndDataPackedInOnePDU_BothMessagesDeliveredInOrder(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	serverTransport := network.NewTransport(serverConn, network.DefaultMaxPDUSize)
	clientTransport := network.NewTransport(clientConn, network.DefaultMaxPDUSize)

	clientAssoc := network.NewAssociation(clientTransport)
	serverAssoc := network.NewAssociation(serverTransport)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	contexts := network.DefaultVerificationContexts()
	supportedAS := map[string]bool{network.VerificationSOPClassUID: true}
	supportedTS := map[string]bool{network.ExplicitVRLittleEndianUID: true, network.ImplicitVRLittleEndianUID: true}

	errCh := make(chan error, 2)
	go func() {
		errCh <- clientAssoc.RequestAssociation(ctx, "SCU", "SCP", contexts, network.DefaultMaxPDUSize)
	}()
	go func() {
		pdu, err := serverTransport.ReadPDU(ctx)
		if err != nil {
			errCh <- err
			return
		}
		rq := pdu.(*network.AssociateRQ)
		errCh <- serverAssoc.AcceptAssociation(ctx, rq, supportedAS, supportedTS, network.DefaultMaxPDUSize)
	}()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("association setup: %v", err)
		}
	}

	commandBytes := []byte("fake command dataset")
	dataBytes := []byte("fake pixel/dataset bytes")

	// Hand-craft ONE PDataTF PDU carrying BOTH messages — the shape a real
	// dcm4che storescu C-STORE sends for a small dataset, and the exact
	// shape the unpatched library discarded everything after the first
	// IsLast PDV for.
	pdu := &network.PDataTF{
		PDVItems: []network.PDVItem{
			{PresentationContextID: 1, IsCommand: true, IsLast: true, Data: commandBytes},
			{PresentationContextID: 1, IsCommand: false, IsLast: true, Data: dataBytes},
		},
	}

	// net.Pipe() is unbuffered/synchronous: WritePDU's own Write call blocks
	// until the server side performs a matching Read, so it must run
	// concurrently with (not awaited before) the server's own
	// ReceivePData calls below, or both sides deadlock waiting on each
	// other.
	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- clientTransport.WritePDU(ctx, pdu)
	}()

	// First call must return the Command message...
	ctxID, data, isCmd, err := serverAssoc.ReceivePData(ctx)
	if err != nil {
		t.Fatalf("first ReceivePData (command): %v", err)
	}
	if !isCmd {
		t.Error("first ReceivePData: expected isCommand=true")
	}
	if string(data) != string(commandBytes) {
		t.Errorf("first ReceivePData: data = %q, want %q", data, commandBytes)
	}
	if ctxID != 1 {
		t.Errorf("first ReceivePData: contextID = %d, want 1", ctxID)
	}

	// ...and the SECOND call must return the Data message — this is the
	// exact call that hung forever before the fix (it used to block on a
	// fresh ReadPDU for bytes that were already consumed and discarded).
	done := make(chan struct{})
	var ctxID2 byte
	var data2 []byte
	var isCmd2 bool
	var err2 error
	go func() {
		ctxID2, data2, isCmd2, err2 = serverAssoc.ReceivePData(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("second ReceivePData hung — the PDV-dropping bug is back")
	}

	if err2 != nil {
		t.Fatalf("second ReceivePData (data): %v", err2)
	}
	if isCmd2 {
		t.Error("second ReceivePData: expected isCommand=false")
	}
	if string(data2) != string(dataBytes) {
		t.Errorf("second ReceivePData: data = %q, want %q", data2, dataBytes)
	}
	if ctxID2 != 1 {
		t.Errorf("second ReceivePData: contextID = %d, want 1", ctxID2)
	}

	if err := <-sendErrCh; err != nil {
		t.Fatalf("WritePDU: %v", err)
	}
}

// TestReceivePData_ThreeMessagesPackedInOnePDU proves the fix generalizes
// beyond exactly two messages — queueRemainingPDVItems must correctly
// split and queue multiple trailing messages, not just one.
func TestReceivePData_ThreeMessagesPackedInOnePDU(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	serverTransport := network.NewTransport(serverConn, network.DefaultMaxPDUSize)
	clientTransport := network.NewTransport(clientConn, network.DefaultMaxPDUSize)

	clientAssoc := network.NewAssociation(clientTransport)
	serverAssoc := network.NewAssociation(serverTransport)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	contexts := network.DefaultVerificationContexts()
	supportedAS := map[string]bool{network.VerificationSOPClassUID: true}
	supportedTS := map[string]bool{network.ExplicitVRLittleEndianUID: true, network.ImplicitVRLittleEndianUID: true}

	errCh := make(chan error, 2)
	go func() {
		errCh <- clientAssoc.RequestAssociation(ctx, "SCU", "SCP", contexts, network.DefaultMaxPDUSize)
	}()
	go func() {
		pdu, err := serverTransport.ReadPDU(ctx)
		if err != nil {
			errCh <- err
			return
		}
		rq := pdu.(*network.AssociateRQ)
		errCh <- serverAssoc.AcceptAssociation(ctx, rq, supportedAS, supportedTS, network.DefaultMaxPDUSize)
	}()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("association setup: %v", err)
		}
	}

	msgs := [][]byte{[]byte("msg-one"), []byte("msg-two"), []byte("msg-three")}
	pdu := &network.PDataTF{
		PDVItems: []network.PDVItem{
			{PresentationContextID: 1, IsCommand: true, IsLast: true, Data: msgs[0]},
			{PresentationContextID: 1, IsCommand: false, IsLast: true, Data: msgs[1]},
			{PresentationContextID: 1, IsCommand: false, IsLast: true, Data: msgs[2]},
		},
	}

	sendErrCh := make(chan error, 1)
	go func() { sendErrCh <- clientTransport.WritePDU(ctx, pdu) }()

	for i, want := range msgs {
		_, data, _, err := serverAssoc.ReceivePData(ctx)
		if err != nil {
			t.Fatalf("ReceivePData call %d: %v", i+1, err)
		}
		if string(data) != string(want) {
			t.Errorf("ReceivePData call %d: data = %q, want %q", i+1, data, want)
		}
	}

	if err := <-sendErrCh; err != nil {
		t.Fatalf("WritePDU: %v", err)
	}
}
