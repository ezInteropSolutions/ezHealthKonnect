const fs = require('fs');
const imgs = require('./imgs_export.js');

const html = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ezHealthKonnect — Go-To-Market One-Pager</title>
<style>
  @import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800;900&display=swap');
  * { margin: 0; padding: 0; box-sizing: border-box; }
  :root {
    --pink:    #ec4899;
    --pink-d:  #db2777;
    --pink-l:  #f472b6;
    --pink-xl: #fce7f3;
    --slate:   #1e293b;
    --gray:    #64748b;
    --light:   #f8fafc;
    --white:   #ffffff;
    --border:  #e2e8f0;
    --shadow:  0 4px 24px rgba(236,72,153,0.10);
  }
  body {
    font-family: 'Inter', system-ui, sans-serif;
    background: var(--white);
    color: var(--slate);
    line-height: 1.6;
    font-size: 14px;
    -webkit-print-color-adjust: exact;
    print-color-adjust: exact;
  }
  .page { max-width: 1100px; margin: 0 auto; padding: 0 48px; }

  /* HEADER */
  header {
    background: linear-gradient(135deg, #1e293b 0%, #0f172a 100%);
    padding: 0;
    position: relative;
    overflow: hidden;
  }
  header::before {
    content: '';
    position: absolute; top: -60px; right: -60px;
    width: 300px; height: 300px;
    background: radial-gradient(circle, rgba(236,72,153,0.25) 0%, transparent 70%);
    border-radius: 50%;
  }
  .header-inner {
    display: flex; align-items: center; justify-content: space-between;
    padding: 24px 48px; position: relative; z-index: 1;
  }
  .header-left { display: flex; align-items: center; gap: 16px; }
  .logo-img { width: 50px; height: 50px; border-radius: 12px; object-fit: cover; border: 2px solid rgba(236,72,153,0.4); }
  .brand-text h1 { color: white; font-size: 22px; font-weight: 800; letter-spacing: -0.5px; }
  .brand-text h1 span { color: #f472b6; }
  .brand-text p { color: #94a3b8; font-size: 12px; margin-top: 2px; }
  .header-badges { display: flex; gap: 10px; align-items: center; }
  .badge { padding: 6px 14px; border-radius: 999px; font-size: 11px; font-weight: 600; letter-spacing: 0.5px; text-transform: uppercase; }
  .badge-hipaa { background: rgba(236,72,153,0.2); color: #f472b6; border: 1px solid rgba(236,72,153,0.3); }
  .badge-hl7   { background: rgba(99,102,241,0.2); color: #a5b4fc; border: 1px solid rgba(99,102,241,0.3); }
  .badge-fhir  { background: rgba(34,197,94,0.2);  color: #86efac; border: 1px solid rgba(34,197,94,0.3); }
  .powered-by { color: #475569; font-size: 11px; text-align: right; }
  .powered-by span { color: #94a3b8; font-weight: 600; display: block; font-size: 13px; }

  /* HERO */
  .hero {
    background: linear-gradient(135deg, #0f172a 0%, #1e293b 50%, #0f172a 100%);
    padding: 64px 0;
  }
  .hero-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 56px; align-items: center; }
  .hero-tag {
    display: inline-flex; align-items: center; gap: 8px;
    background: rgba(236,72,153,0.15); border: 1px solid rgba(236,72,153,0.3);
    color: #f472b6; border-radius: 999px; padding: 6px 16px; font-size: 12px; font-weight: 600;
    margin-bottom: 20px;
  }
  .hero-tag::before { content: ''; width: 6px; height: 6px; background: #ec4899; border-radius: 50%; }
  .hero h2 { color: white; font-size: 38px; font-weight: 900; line-height: 1.15; letter-spacing: -1.5px; margin-bottom: 20px; }
  .hero h2 .hl {
    background: linear-gradient(135deg, #f472b6, #ec4899);
    -webkit-background-clip: text; -webkit-text-fill-color: transparent;
  }
  .hero-sub { color: #94a3b8; font-size: 16px; line-height: 1.7; margin-bottom: 32px; }
  .hero-stats { display: flex; gap: 32px; }
  .stat-num { font-size: 28px; font-weight: 900; color: white; }
  .stat-num span { color: #f472b6; }
  .stat-lbl { font-size: 11px; color: #64748b; text-transform: uppercase; letter-spacing: 0.5px; margin-top: 2px; }
  .hero-img-wrap {
    border-radius: 16px; overflow: hidden; position: relative;
    box-shadow: 0 24px 64px rgba(0,0,0,0.5), 0 0 0 1px rgba(236,72,153,0.2);
  }
  .hero-img-wrap img { width: 100%; display: block; }
  .screen-label {
    position: absolute; bottom: 12px; left: 12px;
    background: rgba(15,23,42,0.85); backdrop-filter: blur(4px);
    color: white; font-size: 11px; font-weight: 500; padding: 4px 10px;
    border-radius: 6px; border: 1px solid rgba(255,255,255,0.1);
  }

  /* SECTIONS */
  section { padding: 60px 0; }
  .section-tag {
    display: inline-flex; align-items: center; gap: 6px;
    color: #ec4899; font-size: 12px; font-weight: 700;
    text-transform: uppercase; letter-spacing: 1px; margin-bottom: 12px;
  }
  .section-tag::before { content: ''; width: 20px; height: 2px; background: #ec4899; border-radius: 1px; }
  h3.section-title { font-size: 28px; font-weight: 800; color: var(--slate); letter-spacing: -0.5px; margin-bottom: 8px; }
  p.section-sub { color: var(--gray); font-size: 15px; max-width: 560px; }

  /* PROBLEM / SOLUTION */
  .problem-solution { background: var(--light); padding: 60px 0; }
  .ps-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 28px; margin-top: 40px; }
  .ps-card { background: white; border-radius: 16px; padding: 28px; border: 1px solid var(--border); box-shadow: var(--shadow); }
  .ps-card.problem { border-top: 3px solid #ef4444; }
  .ps-card.solution { border-top: 3px solid #22c55e; }
  .ps-card h4 { font-size: 15px; font-weight: 700; margin-bottom: 16px; }
  .ps-card.problem h4 { color: #ef4444; }
  .ps-card.solution h4 { color: #16a34a; }
  .ps-list { list-style: none; display: flex; flex-direction: column; gap: 10px; }
  .ps-list li { display: flex; align-items: flex-start; gap: 10px; font-size: 13px; color: var(--gray); line-height: 1.5; }
  .ps-list li .li-icon { flex-shrink: 0; font-size: 16px; }

  /* FEATURES */
  .features-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; margin-top: 40px; }
  .feature-card {
    background: var(--light); border-radius: 14px; padding: 24px;
    border: 1px solid var(--border); position: relative; overflow: hidden;
  }
  .feature-card::before {
    content: ''; position: absolute; top: 0; left: 0; right: 0; height: 3px;
    background: linear-gradient(90deg, #f472b6, #ec4899);
  }
  .feature-icon { font-size: 28px; margin-bottom: 12px; display: block; }
  .feature-card h4 { font-size: 14px; font-weight: 700; color: var(--slate); margin-bottom: 8px; }
  .feature-card p { font-size: 12px; color: var(--gray); line-height: 1.6; }

  /* SCREENSHOTS */
  .screenshots-section { background: var(--light); padding: 60px 0; }
  .screens-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; margin-top: 40px; }
  .screen-card { border-radius: 12px; overflow: hidden; box-shadow: 0 4px 20px rgba(0,0,0,0.1); border: 1px solid var(--border); }
  .screen-card img { width: 100%; display: block; }
  .screen-card-label { background: white; padding: 10px 14px; font-size: 12px; font-weight: 600; color: var(--slate); border-top: 1px solid var(--border); }
  .screen-card-label span { color: var(--gray); font-weight: 400; font-size: 11px; display: block; }

  /* CONNECTORS */
  .connectors-section { background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%); padding: 60px 0; }
  .connectors-section .section-title { color: white; }
  .connectors-section .section-sub { color: #94a3b8; }
  .connectors-section .section-tag { color: #f472b6; }
  .connectors-section .section-tag::before { background: #f472b6; }
  .conn-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-top: 40px; }
  .conn-cat { background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.1); border-radius: 12px; padding: 16px; }
  .conn-cat-title { font-size: 11px; font-weight: 700; color: #f472b6; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 10px; }
  .conn-list { list-style: none; display: flex; flex-direction: column; gap: 6px; }
  .conn-list li { font-size: 12px; color: #cbd5e1; display: flex; align-items: center; gap: 6px; }
  .conn-list li::before { content: ''; width: 5px; height: 5px; background: #ec4899; border-radius: 50%; flex-shrink: 0; }
  .conn-total {
    margin-top: 28px; text-align: center;
    background: linear-gradient(135deg, rgba(236,72,153,0.15), rgba(244,114,182,0.1));
    border: 1px solid rgba(236,72,153,0.3); border-radius: 12px; padding: 20px; color: white;
  }
  .conn-total strong { font-size: 32px; font-weight: 900; color: #f472b6; display: block; }
  .conn-total span { font-size: 13px; color: #94a3b8; }

  /* HOW IT WORKS */
  .flow-steps { display: flex; align-items: center; gap: 0; margin-top: 40px; }
  .flow-step { flex: 1; text-align: center; padding: 24px 12px; }
  .flow-arrow { font-size: 24px; color: #f472b6; flex-shrink: 0; padding: 0 4px; }
  .step-num {
    width: 48px; height: 48px; border-radius: 50%; margin: 0 auto 14px;
    display: flex; align-items: center; justify-content: center; font-size: 18px; font-weight: 800;
    background: linear-gradient(135deg, #ec4899, #db2777); color: white;
    box-shadow: 0 4px 16px rgba(236,72,153,0.3);
  }
  .flow-step .step-icon { font-size: 22px; margin-bottom: 8px; }
  .flow-step h4 { font-size: 13px; font-weight: 700; color: var(--slate); margin-bottom: 6px; }
  .flow-step p { font-size: 12px; color: var(--gray); line-height: 1.5; }

  /* AUDIENCE */
  .audience-section { background: var(--light); padding: 60px 0; }
  .audience-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-top: 40px; }
  .aud-card { background: white; border-radius: 12px; padding: 20px; border: 1px solid var(--border); }
  .aud-icon { font-size: 28px; margin-bottom: 8px; }
  .aud-card h4 { font-size: 13px; font-weight: 700; color: var(--slate); margin-bottom: 6px; }
  .aud-card p { font-size: 12px; color: var(--gray); line-height: 1.5; }

  /* DIFFERENTIATORS */
  .diff-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; margin-top: 40px; }
  .diff-item { display: flex; align-items: flex-start; gap: 14px; padding: 16px; border-radius: 10px; background: var(--light); border: 1px solid var(--border); }
  .diff-icon { width: 36px; height: 36px; border-radius: 8px; display: flex; align-items: center; justify-content: center; font-size: 18px; flex-shrink: 0; background: #fce7f3; }
  .diff-text h4 { font-size: 13px; font-weight: 700; color: var(--slate); margin-bottom: 4px; }
  .diff-text p { font-size: 12px; color: var(--gray); line-height: 1.5; }

  /* FOOTER / CTA */
  footer {
    background: linear-gradient(135deg, #db2777 0%, #ec4899 50%, #f472b6 100%);
    padding: 56px 0; text-align: center; position: relative; overflow: hidden;
  }
  footer h2 { color: white; font-size: 28px; font-weight: 800; margin-bottom: 12px; }
  footer > .page > p { color: rgba(255,255,255,0.85); font-size: 15px; margin-bottom: 28px; }
  .cta-row { display: flex; gap: 16px; justify-content: center; flex-wrap: wrap; }
  .cta-btn { padding: 14px 32px; border-radius: 999px; font-size: 14px; font-weight: 700; text-decoration: none; letter-spacing: 0.3px; border: 2px solid white; }
  .cta-primary { background: white; color: #db2777; }
  .cta-secondary { background: transparent; color: white; }
  .footer-bottom { margin-top: 40px; padding-top: 24px; border-top: 1px solid rgba(255,255,255,0.2); display: flex; justify-content: space-between; align-items: center; }
  .footer-logo { display: flex; align-items: center; gap: 10px; }
  .footer-logo img { width: 32px; height: 32px; border-radius: 8px; object-fit: cover; }
  .footer-logo span { color: white; font-weight: 700; font-size: 14px; }
  .footer-contact { color: rgba(255,255,255,0.7); font-size: 12px; text-align: right; line-height: 1.8; }
  .footer-contact a { color: white; text-decoration: none; }
</style>
</head>
<body>

<!-- ═══ HEADER ═══ -->
<header>
  <div class="header-inner">
    <div class="header-left">
      <img src="${imgs.logo}" alt="ezHealthKonnect Logo" class="logo-img">
      <div class="brand-text">
        <h1>ez<span>Health</span>Konnect</h1>
        <p>by ezInteropSolutions</p>
      </div>
    </div>
    <div class="header-badges">
      <span class="badge badge-hipaa">HIPAA Ready</span>
      <span class="badge badge-hl7">HL7 v2</span>
      <span class="badge badge-fhir">FHIR R4</span>
    </div>
    <div class="powered-by">
      <span>ezInteropSolutions</span>
      Healthcare Integration Platform
    </div>
  </div>
</header>

<!-- ═══ HERO ═══ -->
<div class="hero">
  <div class="page">
    <div class="hero-grid">
      <div>
        <div class="hero-tag">Healthcare Middleware Platform</div>
        <h2>Connect Any System.<br>Transform Any Message.<br><span class="hl">Instantly.</span></h2>
        <p class="hero-sub">
          ezHealthKonnect is an AI-powered integration platform that bridges legacy HL7 systems
          with modern FHIR APIs — without ripping out what already works.
        </p>
        <div class="hero-stats">
          <div class="stat">
            <div class="stat-num">32<span>+</span></div>
            <div class="stat-lbl">OOB Connectors</div>
          </div>
          <div class="stat">
            <div class="stat-num"><span>&lt;</span>1s</div>
            <div class="stat-lbl">Transform Latency</div>
          </div>
          <div class="stat">
            <div class="stat-num">100<span>%</span></div>
            <div class="stat-lbl">On-Premise</div>
          </div>
        </div>
      </div>
      <div>
        <div class="hero-img-wrap">
          <img src="${imgs.dashboard}" alt="Real-Time Dashboard">
          <div class="screen-label">Real-Time Dashboard</div>
        </div>
      </div>
    </div>
  </div>
</div>

<!-- ═══ PROBLEM / SOLUTION ═══ -->
<div class="problem-solution">
  <div class="page">
    <div class="section-tag">The Challenge</div>
    <h3 class="section-title">Healthcare Integration Is Broken</h3>
    <p class="section-sub">Most organizations manage a patchwork of legacy systems that can't talk to each other — costing time, money, and patient outcomes.</p>
    <div class="ps-grid">
      <div class="ps-card problem">
        <h4>&#10060; &nbsp;The Problem</h4>
        <ul class="ps-list">
          <li><span class="li-icon">&#128184;</span>Point-to-point integrations cost $50K–$500K per interface</li>
          <li><span class="li-icon">&#128034;</span>Months of custom dev work for every new connection</li>
          <li><span class="li-icon">&#128274;</span>Siloed HL7 data trapped in legacy systems</li>
          <li><span class="li-icon">&#128203;</span>Manual, error-prone HL7 to FHIR translations</li>
          <li><span class="li-icon">&#9888;</span>No centralized audit trail for HIPAA compliance</li>
          <li><span class="li-icon">&#128257;</span>Vendor lock-in with proprietary integration engines</li>
        </ul>
      </div>
      <div class="ps-card solution">
        <h4>&#10004; &nbsp;ezHealthKonnect Solves This</h4>
        <ul class="ps-list">
          <li><span class="li-icon">&#128640;</span>Deploy in hours with wizard-driven interface setup</li>
          <li><span class="li-icon">&#129302;</span>AI-assisted HL7 to FHIR mapping with smart suggestions</li>
          <li><span class="li-icon">&#128268;</span>32 out-of-box connectors — zero custom code needed</li>
          <li><span class="li-icon">&#128202;</span>Real-time pipeline monitoring and message tracing</li>
          <li><span class="li-icon">&#128737;</span>Built-in HIPAA audit trail and PHI egress gating</li>
          <li><span class="li-icon">&#127968;</span>Runs entirely on your infrastructure — PHI stays put</li>
        </ul>
      </div>
    </div>
  </div>
</div>

<!-- ═══ FEATURES ═══ -->
<section>
  <div class="page">
    <div class="section-tag">Core Capabilities</div>
    <h3 class="section-title">Everything You Need. Nothing You Don't.</h3>
    <p class="section-sub">Built for healthcare IT teams who need real integrations — not more complexity.</p>
    <div class="features-grid">
      <div class="feature-card">
        <span class="feature-icon">&#128257;</span>
        <h4>HL7 to FHIR Transformation</h4>
        <p>Auto-transform ADT, ORU, ORM, MDM, and more. Schema-validated mappings with full field-level control and custom JavaScript logic.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#129497;</span>
        <h4>Visual Pipeline Builder</h4>
        <p>Drag-and-drop pipeline steps: validation, enrichment, conditional routing, data masking, and custom scripts — no IDE required.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#128268;</span>
        <h4>32 OOB Connectors</h4>
        <p>TCP/MLLP, HTTP, File, SFTP, PostgreSQL, MySQL, Oracle, MongoDB, Kafka, RabbitMQ, AWS S3, and more — inbound and outbound.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#128202;</span>
        <h4>Real-Time Monitoring</h4>
        <p>Live interface status, message throughput charts, latency metrics, and error alerting — all in one dashboard.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#128737;</span>
        <h4>HIPAA-Ready Compliance</h4>
        <p>Every message logged, every action audited. PHI detection and masking built into the pipeline. Role-based access control.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#129302;</span>
        <h4>AI-Assisted Mapping</h4>
        <p>Intelligent field suggestions, OOB mapping templates for ADT^A01, ORU^R01 and more. Optional local LLM via ezCompanion.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#128228;</span>
        <h4>Dead Letter Queue</h4>
        <p>Failed messages captured automatically. Inspect, edit, and retry with one click — no messages lost in transit.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#127981;</span>
        <h4>Interface Templates</h4>
        <p>9+ OOB templates for common scenarios: ADT feeds, Lab results, Scheduling. Import a template and go live immediately.</p>
      </div>
      <div class="feature-card">
        <span class="feature-icon">&#128269;</span>
        <h4>Message Inspector</h4>
        <p>Per-interface message viewer with full HL7 parsing, enhanced schema view, raw/parsed toggle, and search filtering.</p>
      </div>
    </div>
  </div>
</section>

<!-- ═══ SCREENSHOTS ═══ -->
<div class="screenshots-section">
  <div class="page">
    <div class="section-tag">In Action</div>
    <h3 class="section-title">See the Platform</h3>
    <p class="section-sub">Real screenshots from a running ezHealthKonnect instance.</p>
    <div class="screens-grid">
      <div class="screen-card">
        <img src="${imgs.interfaces}" alt="Interface Management">
        <div class="screen-card-label">Interface Management<span>Configure, activate, and monitor every integration point</span></div>
      </div>
      <div class="screen-card">
        <img src="${imgs.monitoring}" alt="Real-Time Monitoring">
        <div class="screen-card-label">Real-Time Monitoring<span>Live throughput, latency, and error metrics per interface</span></div>
      </div>
      <div class="screen-card">
        <img src="${imgs.messages}" alt="Message Viewer">
        <div class="screen-card-label">Message Viewer<span>Full HL7 parse tree with field-level inspection</span></div>
      </div>
    </div>
  </div>
</div>

<!-- ═══ CONNECTORS ═══ -->
<div class="connectors-section">
  <div class="page">
    <div class="section-tag">Connectivity</div>
    <h3 class="section-title">32 Out-of-Box Connectors</h3>
    <p class="section-sub">Connect any system — clinical, cloud, or on-premise — without writing integration code.</p>
    <div class="conn-grid">
      <div class="conn-cat">
        <div class="conn-cat-title">Network / API</div>
        <ul class="conn-list">
          <li>TCP/MLLP Inbound</li>
          <li>TCP/MLLP Outbound</li>
          <li>HTTP REST Inbound</li>
          <li>HTTP Outbound</li>
          <li>HTTP FHIR Inbound</li>
          <li>HTTP FHIR Outbound</li>
        </ul>
      </div>
      <div class="conn-cat">
        <div class="conn-cat-title">Databases</div>
        <ul class="conn-list">
          <li>PostgreSQL</li>
          <li>MySQL</li>
          <li>SQL Server</li>
          <li>Oracle</li>
          <li>MongoDB</li>
          <li>Snowflake / BigQuery</li>
        </ul>
      </div>
      <div class="conn-cat">
        <div class="conn-cat-title">Messaging &amp; Files</div>
        <ul class="conn-list">
          <li>Apache Kafka</li>
          <li>RabbitMQ</li>
          <li>Redis</li>
          <li>File Listener</li>
          <li>File Writer</li>
          <li>SFTP Inbound/Outbound</li>
        </ul>
      </div>
      <div class="conn-cat">
        <div class="conn-cat-title">Cloud Storage</div>
        <ul class="conn-list">
          <li>AWS S3</li>
          <li>Azure Blob Storage</li>
          <li>Google Cloud Storage</li>
          <li>FTP Inbound/Outbound</li>
          <li>EDI X12 (Phase 4)</li>
          <li>Direct Messaging (Phase 4)</li>
        </ul>
      </div>
    </div>
    <div class="conn-total">
      <strong>32</strong>
      <span>connectors — 16 inbound + 16 outbound — all configurable without writing code</span>
    </div>
  </div>
</div>

<!-- ═══ HOW IT WORKS ═══ -->
<section>
  <div class="page">
    <div class="section-tag">How It Works</div>
    <h3 class="section-title">From Setup to Live in Hours</h3>
    <p class="section-sub">ezHealthKonnect's wizard-driven flow means your first interface goes live the same day.</p>
    <div class="flow-steps">
      <div class="flow-step">
        <div class="step-num">1</div>
        <div class="step-icon">&#128268;</div>
        <h4>Connect Your Source</h4>
        <p>Pick an inbound connector (MLLP, file, database, queue). Configure host, port, and credentials in the UI.</p>
      </div>
      <div class="flow-arrow">&#8594;</div>
      <div class="flow-step">
        <div class="step-num">2</div>
        <div class="step-icon">&#129497;</div>
        <h4>Define Your Pipeline</h4>
        <p>Use OOB templates or build custom. Add validation, enrichment, HL7 to FHIR mapping, and routing steps.</p>
      </div>
      <div class="flow-arrow">&#8594;</div>
      <div class="flow-step">
        <div class="step-num">3</div>
        <div class="step-icon">&#128228;</div>
        <h4>Route to Destination</h4>
        <p>Pick an outbound connector. Messages are transformed and delivered with full audit trail and retry on failure.</p>
      </div>
      <div class="flow-arrow">&#8594;</div>
      <div class="flow-step">
        <div class="step-num">4</div>
        <div class="step-icon">&#128202;</div>
        <h4>Monitor &amp; Optimize</h4>
        <p>Watch live throughput, inspect individual messages, investigate failures, and tune performance in one place.</p>
      </div>
    </div>
  </div>
</section>

<!-- ═══ WHO IT'S FOR ═══ -->
<div class="audience-section">
  <div class="page">
    <div class="section-tag">Who It's For</div>
    <h3 class="section-title">Built for Healthcare Organizations</h3>
    <p class="section-sub">Any team that moves health data between systems.</p>
    <div class="audience-grid">
      <div class="aud-card">
        <div class="aud-icon">&#127973;</div>
        <h4>Hospital IT Departments</h4>
        <p>Connect your HIS, EHR, lab systems, and ADT feeds — without vendor-proprietary middleware.</p>
      </div>
      <div class="aud-card">
        <div class="aud-icon">&#128300;</div>
        <h4>Labs &amp; Diagnostics</h4>
        <p>Route ORU^R01 lab results to any destination — EHR, physician portal, or HIE — automatically.</p>
      </div>
      <div class="aud-card">
        <div class="aud-icon">&#127970;</div>
        <h4>Health Information Exchanges</h4>
        <p>Ingest from dozens of sources, normalize to FHIR, and publish to any consumer in real time.</p>
      </div>
      <div class="aud-card">
        <div class="aud-icon">&#128188;</div>
        <h4>Revenue Cycle Management</h4>
        <p>Process ADT, DFT, and claim messages at scale with configurable business rules per payer.</p>
      </div>
      <div class="aud-card">
        <div class="aud-icon">&#128187;</div>
        <h4>Healthcare ISVs</h4>
        <p>Embed ezHealthKonnect as your integration engine — white-label ready, Docker-deployable.</p>
      </div>
      <div class="aud-card">
        <div class="aud-icon">&#127963;</div>
        <h4>Public Health Agencies</h4>
        <p>Aggregate reporting from hundreds of facilities using standardized FHIR pipelines.</p>
      </div>
    </div>
  </div>
</div>

<!-- ═══ DIFFERENTIATORS ═══ -->
<section>
  <div class="page">
    <div class="section-tag">Why ezHealthKonnect</div>
    <h3 class="section-title">Different By Design</h3>
    <p class="section-sub">A modern integration engine without the enterprise price tag or the proprietary lock-in.</p>
    <div class="diff-grid">
      <div class="diff-item">
        <div class="diff-icon">&#127968;</div>
        <div class="diff-text">
          <h4>100% On-Premise — PHI Never Leaves</h4>
          <p>Runs in your own Docker environment. No SaaS data sharing, no cloud dependency for message processing.</p>
        </div>
      </div>
      <div class="diff-item">
        <div class="diff-icon">&#9889;</div>
        <div class="diff-text">
          <h4>Go-Powered Performance Engine</h4>
          <p>The transformation core is written in Go — sub-millisecond processing, concurrent pipelines, zero GC pauses.</p>
        </div>
      </div>
      <div class="diff-item">
        <div class="diff-icon">&#127919;</div>
        <div class="diff-text">
          <h4>No-Code for Standard Flows</h4>
          <p>OOB templates cover 80% of common scenarios. Custom logic uses JavaScript — not proprietary scripting languages.</p>
        </div>
      </div>
      <div class="diff-item">
        <div class="diff-icon">&#128176;</div>
        <div class="diff-text">
          <h4>Fraction of the Cost</h4>
          <p>vs. Mirth Connect (requires Go devs), Rhapsody (6-figure licensing), or Azure Health APIs (per-message fees).</p>
        </div>
      </div>
      <div class="diff-item">
        <div class="diff-icon">&#128275;</div>
        <div class="diff-text">
          <h4>No Vendor Lock-In</h4>
          <p>Open connector framework. If we don't have your connector, you can implement the interface — it's standard Go.</p>
        </div>
      </div>
      <div class="diff-item">
        <div class="diff-icon">&#128640;</div>
        <div class="diff-text">
          <h4>Same-Day Go-Live</h4>
          <p>Install via Docker in under 30 minutes. First interface live in hours — not the weeks typical of enterprise middleware.</p>
        </div>
      </div>
    </div>
  </div>
</div>

<!-- ═══ FOOTER / CTA ═══ -->
<footer>
  <div class="page">
    <h2>Ready to Modernize Your Healthcare Integrations?</h2>
    <p>Book a demo or request a 30-day trial — we'll have your first interface live before the call ends.</p>
    <div class="cta-row">
      <a href="mailto:hello@ezinteropsolutions.com" class="cta-btn cta-primary">Request a Demo</a>
      <a href="mailto:hello@ezinteropsolutions.com" class="cta-btn cta-secondary">Get a Trial License</a>
    </div>
    <div class="footer-bottom">
      <div class="footer-logo">
        <img src="${imgs.logo}" alt="logo">
        <span>ezHealthKonnect by ezInteropSolutions</span>
      </div>
      <div class="footer-contact">
        <a href="mailto:hello@ezinteropsolutions.com">hello@ezinteropsolutions.com</a><br>
        &copy; 2025 ezInteropSolutions. All rights reserved. &nbsp;|&nbsp; HIPAA-Compliant Platform
      </div>
    </div>
  </div>
</footer>

</body>
</html>`;

fs.writeFileSync('ezHealthKonnect-GTM-OnePager.html', html);
console.log('Written: marketing/ezHealthKonnect-GTM-OnePager.html');
console.log('Size:', (html.length / 1024).toFixed(1) + ' KB');
