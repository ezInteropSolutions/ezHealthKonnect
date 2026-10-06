// dicom/receiver.go
// A DICOM Storage SCP: answers C-ECHO (verification) and C-STORE (image
// receive) only — this phase's own, explicit scope boundary. No Modality
// Worklist, no MPPS, no Storage Commitment, no pixel-data decoding (this is
// middleware, not a PACS — pixel data travels as opaque bytes inside the
// returned Part 10 file). Every other DIMSE service is refused at
// association negotiation by the curated abstract-syntax whitelist in
// Start, not silently mishandled by a default handler.
package dicom

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/amrshadid/go-dicom/fileutil"
	"github.com/amrshadid/go-dicom/network"
)

// ReceiverConfig configures a Receiver.
type ReceiverConfig struct {
	AETitle     string
	Port        int
	BindAddress string

	// MaxAssociations bounds concurrent associations (0 = unlimited) —
	// enforced natively by the library's own semaphore, not re-implemented
	// here.
	MaxAssociations int

	// AllowedCallingAETitles is checked inside every DIMSE handler (there is
	// no pre-association accept/reject hook in this library). Empty/unset
	// means "accept any", matching this codebase's own permissive-by-default
	// convention for optional allowlist arrays elsewhere.
	AllowedCallingAETitles []string

	// AdditionalAbstractSyntaxes extends the built-in Verification/CR/DX/
	// Secondary-Capture whitelist with extra SOP Class UIDs this library has
	// no named constant for.
	AdditionalAbstractSyntaxes []string

	// MaxInstanceSizeBytes bounds the serialized Part 10 size of ONE
	// received instance (0 = no app-level bound, beyond whatever go-dicom's
	// own internal PDU-length ceilings allow — confirmed by reading
	// network/server.go/dimse.go: a hard 128 MiB cap for uncompressed PDVs,
	// 256 MiB for deflate-compressed ones, neither configurable). A
	// misbehaving or malicious sender otherwise has nothing bounding it
	// below that ceiling, and nothing stops a legitimately oversized study
	// from being stored and forwarded downstream. Enforced AFTER
	// SerializeToPart10 (the library has already reassembled the dataset
	// into memory by the time HandleCStore runs — this cannot prevent that
	// one transfer's own peak memory use, only stop the oversized object
	// from being stored/forwarded). Found during a 360 QA pass (October
	// 2026) as a real, previously-named-but-unaddressed gap.
	MaxInstanceSizeBytes int64

	// TLSEnabled/TLSCertFile/TLSKeyFile configure server-side transport TLS
	// (encrypts the channel, authenticates the server) — mirrors
	// as2_inbound.go's own convention exactly.
	TLSEnabled  bool
	TLSCertFile string
	TLSKeyFile  string

	// TLSCAFile, when set alongside TLSEnabled, enables real mutual TLS —
	// a connecting peer must present a certificate signed by this CA, or
	// the TLS handshake itself fails before any DIMSE traffic is possible.
	//
	// An earlier version of this comment said mutual TLS was impossible:
	// go-dicom v1.6.0's own TLSConfig.CAFile field is declared but never
	// read by its own buildTLSConfig (confirmed directly in network/tls.go)
	// — true, but that function ALSO has a real escape hatch one field
	// over: "Config *tls.Config — allows providing a custom *tls.Config
	// directly. If set, CertFile/KeyFile/CAFile are ignored." Start uses
	// that hatch to hand-build a real tls.Config with
	// ClientAuth: RequireAndVerifyClientCert whenever TLSCAFile is set,
	// rather than the broken convenience fields — closing a gap found
	// during a 360 QA pass (October 2026), not a new library capability.
	TLSCAFile string

	// OnInstanceStored is called for each successfully received, allowed
	// C-STORE. Its return value becomes the DIMSE status sent back to the
	// peer — e.g. a caller can return network.StatusOutOfResources if its
	// own downstream queue is full, rather than this package guessing.
	OnInstanceStored func(ctx context.Context, inst *StoredInstance) uint16

	// OnRejected is called whenever a C-ECHO or C-STORE is refused for any
	// reason (AE title not on the allowlist, no concurrency slot available,
	// an invalid peer-supplied SOP Instance UID, or a Part 10 serialization
	// failure) — before this hook existed, every one of these cases sent
	// the correct DIMSE refusal status back to the peer but left zero trace
	// anywhere in this application: no log line, no message row (a rejected
	// C-STORE never reaches OnInstanceStored), nothing. A site's own support
	// engineer troubleshooting "the modality can't connect" would find
	// nothing to look at. callingAE is "" when no association info is
	// available at all (should not normally happen). Optional — nil is a
	// safe no-op.
	OnRejected func(ctx context.Context, callingAE, reason string)
}

// StoredInstance is handed to ReceiverConfig.OnInstanceStored for each
// received instance.
type StoredInstance struct {
	CallingAE      string
	SOPClassUID    string
	SOPInstanceUID string
	Part10         []byte
	Metadata       *InstanceMetadata
}

// Receiver is a thin, reusable wrapper around go-dicom's network package —
// all real protocol logic (handshake, DIMSE dispatch, association
// negotiation) lives in that library; this type owns only the
// application-level decisions this codebase needs (AE allowlisting, Part 10
// serialization, metadata extraction) that the library has no hook to make
// for us.
type Receiver struct {
	cfg    ReceiverConfig
	server *network.Server
}

// NewReceiver constructs a Receiver. Call Start to actually bind and serve.
func NewReceiver(cfg ReceiverConfig) *Receiver {
	return &Receiver{cfg: cfg}
}

// Start binds and begins serving immediately. network.StartServer/
// StartServerTLS are both non-blocking and return a real bind error
// synchronously — unlike network.SCP.ListenAndServe, which blocks and would
// only surface a bind failure from inside a caller's own goroutine.
func (r *Receiver) Start(ctx context.Context) error {
	allowed := make(map[string]bool, len(r.cfg.AllowedCallingAETitles))
	for _, ae := range r.cfg.AllowedCallingAETitles {
		allowed[ae] = true
	}

	handler := &dicomHandler{
		allowed: allowed, onStore: r.cfg.OnInstanceStored, onRejected: r.cfg.OnRejected,
		maxInstanceSizeBytes: r.cfg.MaxInstanceSizeBytes,
	}
	if r.cfg.MaxAssociations > 0 {
		handler.opSlots = make(chan struct{}, r.cfg.MaxAssociations)
	}

	// MaxAssociations is deliberately NOT set on SCPConfig below: confirmed
	// by reading network/server.go in full that StartServer/StartServerTLS
	// run their own accept loop (calling scp.handleConnection directly)
	// rather than delegating to SCP.ListenAndServe, and that loop omits the
	// assocSlots semaphore ListenAndServe's own accept loop has — a real,
	// confirmed gap in go-dicom v1.6.0, not a misunderstanding. Setting the
	// field here would silently do nothing and could mislead a future
	// reader into thinking the library enforces it. The bound is enforced
	// instead by dicomHandler.opSlots, around DIMSE operation processing
	// rather than raw TCP-level association accept — arguably the more
	// operationally relevant protection anyway, since an idle association
	// costs a goroutine and a socket, while concurrent C-STORE processing
	// is where real memory is spent reassembling datasets.
	scpConfig := network.SCPConfig{
		AETitle:     r.cfg.AETitle,
		Port:        r.cfg.Port,
		BindAddress: r.cfg.BindAddress,
	}

	var server *network.Server
	var err error
	if r.cfg.TLSEnabled {
		var tlsCfg *network.TLSConfig
		if r.cfg.TLSCAFile != "" {
			tlsCfg, err = buildMutualTLSConfig(r.cfg.TLSCertFile, r.cfg.TLSKeyFile, r.cfg.TLSCAFile)
			if err != nil {
				return fmt.Errorf("dicom: build mutual TLS config: %w", err)
			}
		} else {
			tlsCfg = &network.TLSConfig{CertFile: r.cfg.TLSCertFile, KeyFile: r.cfg.TLSKeyFile}
		}
		server, err = network.StartServerTLS(ctx, scpConfig, handler, tlsCfg)
	} else {
		server, err = network.StartServer(ctx, scpConfig, handler)
	}
	if err != nil {
		return fmt.Errorf("dicom: start server: %w", err)
	}

	// Restricted to Verification + storage SOP classes this phase actually
	// handles. The library's own default set also includes Query/Retrieve
	// and Storage Commitment, which are NOT auto-withdrawn just because
	// this handler doesn't implement them — refusing their presentation
	// contexts here, at negotiation time, is how this phase's own scope
	// boundary is enforced at the protocol level rather than by omission.
	abstractSyntaxes := append([]string{
		network.VerificationSOPClassUID,
		network.CRImageStorageUID,
		network.DigitalXRayImageStorageUID,
		network.SecondaryCaptureImageStorageUID,
	}, r.cfg.AdditionalAbstractSyntaxes...)
	server.SetSupportedAbstractSyntaxes(abstractSyntaxes)

	// A storage SCP's job is to keep what it is sent — it does not need to
	// decode pixel data to store it, so compressed transfer syntaxes
	// (JPEG/JPEG-LS/JPEG 2000, used natively by most real modalities) must
	// be accepted too, not just the 4 uncompressed defaults. Mirrors
	// go-dicom's own cli/storescp.go reference implementation's documented
	// reasoning for this exact call.
	server.SetSupportedTransferSyntaxes(network.AllTransferSyntaxes())

	r.server = server
	return nil
}

// buildMutualTLSConfig hand-builds a real *tls.Config requiring and
// verifying a client certificate signed by caFile — using
// network.TLSConfig's own "Config *tls.Config" escape hatch (confirmed
// directly in network/tls.go: "If set, CertFile/KeyFile/CAFile are
// ignored"), since the library's own CAFile convenience field is declared
// but never actually read by its buildTLSConfig.
func buildMutualTLSConfig(certFile, keyFile, caFile string) (*network.TLSConfig, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid certificates found in CA file %q", caFile)
	}

	return &network.TLSConfig{
		Config: &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientCAs:    pool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			MinVersion:   tls.VersionTLS12,
		},
	}, nil
}

// Stop stops the server. Safe to call even if Start was never called or
// failed.
func (r *Receiver) Stop() {
	if r.server != nil {
		r.server.Stop()
	}
}

// Addr returns the real bound address (host:port); empty before Start.
func (r *Receiver) Addr() string {
	if r.server == nil {
		return ""
	}
	return r.server.Addr()
}

// dicomHandler implements network.Handler for C-ECHO and C-STORE only.
// Embedding BaseHandler gives every other DIMSE service a safe default,
// backstopped by the abstract-syntax whitelist in Start (which refuses
// those presentation contexts before any such message could ever arrive).
type dicomHandler struct {
	network.BaseHandler
	allowed    map[string]bool
	onStore    func(ctx context.Context, inst *StoredInstance) uint16
	onRejected func(ctx context.Context, callingAE, reason string)

	// opSlots bounds concurrent C-STORE processing when MaxAssociations > 0
	// — see Start's own comment for why this is enforced here rather than
	// via SCPConfig.MaxAssociations. Nil (unbounded) when MaxAssociations
	// is 0/unset. Deliberately NOT applied to HandleCEcho: verification is
	// cheap and stateless, and should stay answerable even while this
	// server is at its C-STORE concurrency limit.
	opSlots chan struct{}

	// maxInstanceSizeBytes — see ReceiverConfig.MaxInstanceSizeBytes's own
	// doc comment. 0 = no app-level bound.
	maxInstanceSizeBytes int64
}

// tryAcquireSlot reports whether a C-STORE processing slot was acquired —
// always true when opSlots is nil (unbounded). Never blocks.
func (h *dicomHandler) tryAcquireSlot() bool {
	if h.opSlots == nil {
		return true
	}
	select {
	case h.opSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

// releaseSlot releases a slot acquired by tryAcquireSlot. Safe to call
// even when opSlots is nil.
func (h *dicomHandler) releaseSlot() {
	if h.opSlots != nil {
		<-h.opSlots
	}
}

// checkAllowed reports whether this association's calling AE title is
// permitted. No pre-association accept/reject hook exists in this library,
// so the check runs inside each DIMSE handler instead of once at
// association time.
func (h *dicomHandler) checkAllowed(ctx context.Context) bool {
	if len(h.allowed) == 0 {
		return true
	}
	info := network.AssociationInfoFromContext(ctx)
	if info == nil {
		return false
	}
	return h.allowed[info.CallingAE]
}

// callingAEFromContext returns the association's calling AE title, or ""
// when no association info is available.
func callingAEFromContext(ctx context.Context) string {
	if info := network.AssociationInfoFromContext(ctx); info != nil {
		return info.CallingAE
	}
	return ""
}

// reject reports a refusal via OnRejected (a no-op if unset) and returns the
// given status, so every rejection branch stays a one-line call.
func (h *dicomHandler) reject(ctx context.Context, reason string) {
	if h.onRejected != nil {
		h.onRejected(ctx, callingAEFromContext(ctx), reason)
	}
}

// HandleCEcho answers verification. A disallowed caller is refused here
// too, not just on C-STORE — an AE that isn't on the allowlist shouldn't
// even get a successful ping.
func (h *dicomHandler) HandleCEcho(ctx context.Context, req *network.CEchoRequest) (*network.CEchoResponse, error) {
	status := network.StatusSuccess
	if !h.checkAllowed(ctx) {
		status = network.StatusRefusedNotAuthorized
		h.reject(ctx, "calling AE title not on the configured allowlist (C-ECHO)")
	}
	return &network.CEchoResponse{
		MessageIDRespondedTo: req.MessageID,
		AffectedSOPClass:     req.AffectedSOPClass,
		Status:               status,
	}, nil
}

// HandleCStore validates the caller and the peer-supplied SOP Instance UID,
// serializes the received dataset to a real Part 10 file, extracts
// metadata, and hands both to the configured OnInstanceStored callback.
func (h *dicomHandler) HandleCStore(ctx context.Context, req *network.CStoreRequest) (*network.CStoreResponse, error) {
	resp := &network.CStoreResponse{
		MessageIDRespondedTo: req.MessageID,
		AffectedSOPClass:     req.AffectedSOPClass,
		AffectedSOPInstance:  req.AffectedSOPInstance,
	}

	if !h.checkAllowed(ctx) {
		resp.Status = network.StatusRefusedNotAuthorized
		h.reject(ctx, "calling AE title not on the configured allowlist (C-STORE)")
		return resp, nil
	}

	if !h.tryAcquireSlot() {
		resp.Status = network.StatusOutOfResources
		h.reject(ctx, "no concurrency slot available (max_associations reached)")
		return resp, nil
	}
	defer h.releaseSlot()

	// req.AffectedSOPInstance arrives from the network peer and is used
	// below as a downstream message identifier — validated with the
	// library's own proven UID-as-identifier check before being trusted
	// for anything, the same "reuse proven security logic, don't reinvent
	// it" discipline already applied elsewhere in this codebase.
	if err := fileutil.ValidateUIDForPath(req.AffectedSOPInstance); err != nil {
		resp.Status = network.StatusInvalidObjectInstance
		h.reject(ctx, fmt.Sprintf("invalid SOP Instance UID %q: %v", req.AffectedSOPInstance, err))
		return resp, nil
	}

	part10, err := SerializeToPart10(req.AffectedSOPClass, req.AffectedSOPInstance, req.DataSet)
	if err != nil {
		resp.Status = network.StatusUnableToProcess
		h.reject(ctx, fmt.Sprintf("Part 10 serialization failed: %v", err))
		return resp, nil
	}

	if h.maxInstanceSizeBytes > 0 && int64(len(part10)) > h.maxInstanceSizeBytes {
		resp.Status = network.StatusOutOfResources
		h.reject(ctx, fmt.Sprintf("instance %s rejected: %d bytes exceeds the configured %d byte limit",
			req.AffectedSOPInstance, len(part10), h.maxInstanceSizeBytes))
		return resp, nil
	}

	inst := &StoredInstance{
		CallingAE:      callingAEFromContext(ctx),
		SOPClassUID:    req.AffectedSOPClass,
		SOPInstanceUID: req.AffectedSOPInstance,
		Part10:         part10,
		Metadata:       ExtractMetadata(req.DataSet),
	}

	if h.onStore != nil {
		resp.Status = h.onStore(ctx, inst)
	} else {
		resp.Status = network.StatusSuccess
	}
	return resp, nil
}
