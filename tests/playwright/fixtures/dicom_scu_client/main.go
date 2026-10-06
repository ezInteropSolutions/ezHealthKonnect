// dicom_scu_client — a small, permanent test fixture that plays the real
// modality's (Storage SCU) role against a real, running ezHealthKonnect
// dicom_storage_inbound connector: Associate, C-ECHO (verification), then
// C-STORE a minimal real dataset.
//
// Promoted from a throwaway full-stack-verification script used during the
// DICOM Storage SCP connector's own implementation (see CLAUDE.md's "Phase
// 2 — Fujifilm DICOM Storage SCP" section) — kept here because
// tests/playwright/dicom-wizard-e2e.spec.js needs the same real DIMSE
// client and no lightweight JS library speaks the real wire protocol (only
// file-parsing libraries like dcmjs exist in the npm ecosystem; hand-rolling
// DICOM's own association negotiation in JS, unlike ASTM's simple ENQ/ACK
// framing, is not practical).
//
// Invoked from the Playwright spec via `docker run --network host
// golang:1.25-alpine go run main.go -addr=... -ae=...` — never run directly
// on the host, per this repo's own standing "Go build/run only inside
// Docker" rule.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
	"github.com/amrshadid/go-dicom/network"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:11112", "host:port of the real running SCP")
	aeTitle := flag.String("ae", "EZHEALTHKONNECT", "called AE title")
	callingAE := flag.String("calling-ae", "FUJIFILM_CR", "calling AE title (this client's own identity)")
	patientID := flag.String("patient-id", "PWTEST001", "PatientID to store")
	flag.Parse()

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: *callingAE, CalledAE: *aeTitle, Address: *addr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := scu.Associate(ctx, nil); err != nil {
		log.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())
	fmt.Println("ASSOCIATED")

	if err := scu.Echo(ctx); err != nil {
		log.Fatalf("Echo: %v", err)
	}
	fmt.Println("ECHO_OK")

	ds := dataset.NewDataset()
	must := func(err error) {
		if err != nil {
			log.Fatalf("AddByKeyword: %v", err)
		}
	}
	must(ds.AddByKeyword("SOPClassUID", dataelem.UI, []byte(network.CRImageStorageUID)))
	must(ds.AddByKeyword("SOPInstanceUID", dataelem.UI, []byte(fmt.Sprintf("1.2.826.0.1.3680043.8.498.%d", time.Now().UnixNano()))))
	must(ds.AddByKeyword("PatientID", dataelem.LO, []byte(*patientID)))
	must(ds.AddByKeyword("PatientName", dataelem.PN, []byte("PlaywrightTest^Device")))
	must(ds.AddByKeyword("StudyInstanceUID", dataelem.UI, []byte("1.2.826.0.1.3680043.8.498.1")))
	must(ds.AddByKeyword("SeriesInstanceUID", dataelem.UI, []byte("1.2.826.0.1.3680043.8.498.2")))
	must(ds.AddByKeyword("Modality", dataelem.CS, []byte("CR")))
	must(ds.AddByKeyword("AccessionNumber", dataelem.SH, []byte("ACC-PW-001")))

	if err := scu.Store(ctx, ds); err != nil {
		log.Fatalf("Store: %v", err)
	}
	fmt.Println("STORE_OK")
}
