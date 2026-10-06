# Local patches to github.com/amrshadid/go-dicom v1.6.0

This directory is a local fork of go-dicom v1.6.0, wired in via a `replace`
directive in the main repo's `go.mod`. It is otherwise byte-identical to the
real upstream module — only the one function below (plus two small,
directly-related support changes) was touched. If upstream ever ships a fix
for this, remove the `replace` directive and this directory.

## Patch 1 — `network/association.go`: `Association.ReceivePData` silently
## dropped PDVs that followed the first `IsLast` PDV within one PDU

### Found
During a 360 QA / production-readiness pass (October 2026) on the
`dicom_storage_inbound` connector, with no real imaging device available to
test against. Independent conformance testing against `dicm4che/storescu`
(a mature, industry-standard, non-go-dicom DICOM toolkit — chosen precisely
because it would NOT share any bug this library's own SCU client might also
have) found that C-ECHO worked perfectly, but **every C-STORE hung
indefinitely** — reproduced with and without PixelData present, ruling out
anything content-specific.

### Root cause
`ReceivePData`'s own PDV-reassembly loop:
```go
for _, pdv := range dataTF.PDVItems {
    assembled = append(assembled, pdv.Data...)
    if pdv.IsLast {
        return contextID, assembled, isCommand, nil
    }
}
```
treats every PDV in one `PDataTF` PDU's `PDVItems` slice as belonging to the
SAME message, and returns as soon as it sees ANY `IsLast` PDV. But DICOM
PS3.8 allows one PDU to carry PDVs from **more than one logical message** —
e.g. the Command message's own final PDV immediately followed, in the SAME
PDU, by the start (or all) of the Data message's own PDV(s). `dcm4che`'s
real encoder does exactly this for a small dataset. The original code
returns on the Command PDV's own `IsLast` and never looks at the remaining
PDV(s) in the same `dataTF.PDVItems` slice — those bytes are gone the moment
the function returns (the whole PDU was already read and parsed; there's
nothing left on the wire to re-read). The caller's NEXT `ReceivePData` call
(reading the dataset) then blocks on a fresh `ReadPDU`, waiting for bytes
that were already consumed and discarded — hanging until the peer's own
read/accept timeout gives up and closes the connection (surfacing as the
EOF this patch's own commit message traces back to).

### Fix
`ReceivePData` now checks, after returning on an `IsLast` PDV, whether more
PDVs remain in the same `dataTF.PDVItems` slice. If so, a new helper,
`queueRemainingPDVItems`, reassembles them (there can be more than one
trailing message) and queues each one via the library's own pre-existing
`a.pending` mechanism (previously used only by `PushBack`, for a C-CANCEL
watcher's own read-ahead) — a later `ReceivePData` call drains that queue
first, exactly as it already did for `PushBack`-pushed messages.

A message queued this way may itself be **incomplete** (its remaining PDU
is still in flight) — `pendingMessage` gained a `complete bool` field for
this; `ReceivePData`'s own pending-check now resumes accumulating an
incomplete entry (seeding `assembled`/`contextID`/`isCommand` and continuing
the normal read loop) rather than returning it immediately. `PushBack`
always sets `complete: true`, matching its own existing contract ("a
message", never a fragment).

### Scope / what this does NOT fix
This patch is scoped to the one confirmed, reproduced failure mode
(multiple messages packed into one PDU). It does not change PDV-encoding on
the SEND side, does not touch any other DIMSE service's own logic beyond
reusing the same shared `ReceivePData`, and makes no claim about covering
every theoretically possible PDV-interleaving pattern — only the one real
senders (confirmed: dcm4che) actually produce.

### Verification
See `dicom/pdv_interop_test.go` in the main repo — a focused test using a
raw `net.Pipe()` to hand-craft a single `PDataTF` PDU containing a
Command PDV immediately followed by a Data PDV (reproducing the exact shape
dcm4che sends), proving `ReceivePData` now returns both messages correctly
across two calls instead of the second one hanging. Also re-verified against
a real `dcm4che/dcm4che-tools` `storescu` C-STORE through the real rebuilt
connector — a real, independent, third-party DICOM toolkit, not this
library testing against itself.
