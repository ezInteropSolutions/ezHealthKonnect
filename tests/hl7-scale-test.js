'use strict';
const net = require('net');
const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');

const CONFIG = {
  mlllpHost:        'localhost',
  mlllpPort:        6613,
  nodeBase:         'http://localhost:3000',
  goBase:           'http://localhost:8080',
  adminEmail:       'admin@ezhealthkonnect.com',
  adminPassword:    'admin123',
  inputInterfaceId: 'df5c7bc7-5dbe-4c47-8298-a4729b26ebd4',
  outputInterfaceId:'05e7d40d-7e36-40fc-a943-9584edc8cbe0',
  socketTimeoutMs:  7000,
  resultsDir:       path.join(__dirname, 'hl7-scale-test-results'),
};

const args = process.argv.slice(2);
const getArg = (flag, def) => { const i = args.indexOf(flag); return i >= 0 && args[i+1] ? args[i+1] : def; };
const FILTER_TYPES  = getArg('--types', '').split(',').filter(Boolean);
const REPEAT_EACH   = parseInt(getArg('--repeat', '1'), 10);
const WAIT_SECS     = parseInt(getArg('--wait', '10'), 10);
const VALID_LEVEL   = getArg('--level', 'standard');
const CONCURRENT    = Math.max(1, parseInt(getArg('--concurrent', '1'), 10));

const REQUIRED = {
  'ADT^A01': ['Patient','Encounter','MessageHeader'],
  'ADT^A03': ['Patient','Encounter','MessageHeader'],
  'ADT^A04': ['Patient','Encounter','MessageHeader'],
  'ADT^A08': ['Patient','Encounter','MessageHeader'],
  'ADT^A28': ['Patient','MessageHeader'],
  'ADT^A39': ['Patient','MessageHeader'],
  'ORU^R01': ['Patient','DiagnosticReport','Observation','MessageHeader'],
  'ORU^R03': ['Patient','DiagnosticReport','Observation','MessageHeader'],
  'ORM^O01': ['Patient','ServiceRequest','MessageHeader'],
  'MDM^T02': ['Patient','DocumentReference','MessageHeader'],
  'VXU^V04': ['Patient','Immunization','MessageHeader'],
  'SIU^S12': ['Patient','Appointment','MessageHeader'],
  'DFT^P03': ['Patient','Encounter','MessageHeader'],
  'MFN^M13': ['MessageHeader'],
};


function buildCorpus() {
  const ts = '20260523', now = ts + '120000', SEG = '\r';
  const msh = (app, type, ctrl) => `MSH|^~\\&|${app}|HOSP|EZHK|EZHK|${now}||${type}|${ctrl}|P|2.5`;
  const corpus = [];

  // ADT^A01 (5 variants)
  const a01p = [
    'PID|1||PAT001^^^HOSP^MR||SMITH^JOHN^Q|||M|||123 MAIN ST^^ANYTOWN^CA^90210',
    'PID|1||PAT002^^^HOSP^MR||JOHNSON^MARY^A|||F|||456 OAK AVE^^SPRINGFIELD^IL^62701',
    'PID|1||PAT003^^^HOSP^MR||WILLIAMS^ROBERT|||M|||789 PINE RD^^CHICAGO^IL^60601',
    'PID|1||PAT004^^^HOSP^MR||DAVIS^LINDA^K|||F|||321 ELM ST^^AURORA^CO^80010',
    'PID|1||PAT005^^^HOSP^MR||MARTINEZ^CARLOS|||M|||654 MAPLE DR^^DENVER^CO^80202',
  ];
  const a01v = [
    'PV1|1|I|WARD01^101^A^HOSP|||DR001^JONES^WILLIAM^^MD||MED|||ADM',
    'PV1|1|I|ICU^201^B^HOSP|||DR003^PATEL^ANIL^^MD||CARD|||ADM',
    'PV1|1|E|ED^301^C^HOSP|||DR005^BROWN^JAMES^^MD||EMER|||ADM',
    'PV1|1|I|ONCO^401^A^HOSP|||DR006^GARCIA^MARIA^^MD||ONC|||ADM',
    'PV1|1|O|OPD^501^D^HOSP|||DR007^WILSON^PAUL^^MD||SURG|||ADM',
  ];
  for (let i = 0; i < 5; i++) {
    const ctrl = 'ADTA01' + String(i+1).padStart(4,'0');
    corpus.push({ type:'ADT^A01', label:`Inpatient admit #${i+1}`, message:
      [msh('EPIC','ADT^A01^ADT_A01',ctrl), 'EVN|A01|'+now, a01p[i], a01v[i],
       'DG1|1|ICD10|J18.9|PNEUMONIA|'+now+'|A'].join(SEG) });
  }

  // ADT^A03 (3)
  for (let i=1;i<=3;i++) {
    const ctrl='ADTA03'+String(i).padStart(4,'0');
    corpus.push({type:'ADT^A03',label:`Discharge #${i}`,message:
      [msh('EPIC','ADT^A03^ADT_A03',ctrl),`EVN|A03|${now}`,
       `PID|1||DPAT00${i}^^^HOSP^MR||DISCHARGE^PAT^${i}|||M`,
       `PV1|1|I|WARD01^10${i}^A^HOSP||||||||DIS`].join(SEG)});
  }

  // ADT^A04 (3)
  for (let i=1;i<=3;i++) {
    const ctrl='ADTA04'+String(i).padStart(4,'0');
    corpus.push({type:'ADT^A04',label:`Outpatient #${i}`,message:
      [msh('CERNER','ADT^A04^ADT_A01',ctrl),`EVN|A04|${now}`,
       `PID|1||OPAT00${i}^^^CLINIC^MR||OUTPT^JANE^${i}|||F|||100 HEALTH WAY^^BOSTON^MA^02101`,
       `PV1|1|O|CLINIC^C0${i}^A^CLINIC|||DR00${i}^PROV^FIRST^^MD|||ALLERGY`].join(SEG)});
  }

  // ADT^A08 (2)
  for (let i=1;i<=2;i++) {
    const ctrl='ADTA08'+String(i).padStart(4,'0');
    corpus.push({type:'ADT^A08',label:`Update #${i}`,message:
      [msh('EPIC','ADT^A08^ADT_A01',ctrl),`EVN|A08|${now}`,
       `PID|1||PAT00${i}^^^HOSP^MR||UPDATED^NAME^${i}|||M|||NEW ADDR^^NEWTOWN^NY^10001`,
       `PV1|1|I|WARD01^10${i}^A^HOSP`].join(SEG)});
  }

  // ADT^A28 (2)
  for (let i=1;i<=2;i++) {
    const ctrl='ADTA28'+String(i).padStart(4,'0');
    corpus.push({type:'ADT^A28',label:`Add person #${i}`,message:
      [msh('EPIC','ADT^A28^ADT_A05',ctrl),`EVN|A28|${now}`,
       `PID|1||NEW00${i}^^^HOSP^MR||NEWPERSON^TEST^${i}|||M|||500 REGISTER LN^^MIAMI^FL^33101`].join(SEG)});
  }

  // ADT^A39 (1)
  corpus.push({type:'ADT^A39',label:'Merge patient',message:
    [msh('EPIC','ADT^A39^ADT_A39','ADTA390001'),`EVN|A39|${now}`,
     `PID|1||PAT010^^^HOSP^MR~PAT011^^^HOSP^MR||MERGED^PATIENT|||M`,
     `MRG|PAT011^^^HOSP^MR||V010`].join(SEG)});

  // ORU^R01 (5)
  const panels = [
    ['OBR|1|ORD001|LAB001|58410-2^CBC^LN|||'+now,'OBX|1|NM|718-7^HEMOGLOBIN^LN||13.5|g/dL|12.0-17.5||||F|||'+now,'OBX|2|NM|777-3^PLATELETS^LN||245|10*3/uL|150-400||||F|||'+now],
    ['OBR|1|ORD002|LAB002|24323-8^METABOLIC^LN|||'+now,'OBX|1|NM|2345-7^GLUCOSE^LN||320|mg/dL|70-100|HH|||F|||'+now,'OBX|2|NM|2160-0^CREATININE^LN||1.8|mg/dL|0.6-1.2|H|||F|||'+now],
    ['OBR|1|ORD003|LAB003|600-7^BLOOD CULTURE^LN|||'+now,'OBX|1|CWE|664-3^GRAM STAIN^LN||260373001^Detected^SCT||POSITIVE|||F|||'+now],
    ['OBR|1|ORD004|LAB004|55230-3^THYROID^LN|||'+now,'OBX|1|NM|3016-3^TSH^LN||0.02|mIU/L|0.4-4.0|LL|||F|||'+now,'OBX|2|NM|3026-2^FREE T4^LN||2.8|ng/dL|0.8-1.8|H|||F|||'+now],
    ['OBR|1|ORD005|LAB005|24357-6^URINALYSIS^LN|||'+now,'OBX|1|NM|2965-2^SP GRAVITY^LN||1.020||1.001-1.030||||F|||'+now,'OBX|2|CWE|25428-4^GLUCOSE UA^LN||Negative||||F|||'+now],
  ];
  for (let i=0;i<5;i++) {
    const ctrl='ORUR01'+String(i+1).padStart(4,'0');
    corpus.push({type:'ORU^R01',label:`Lab result #${i+1}`,message:
      [msh('LIS','ORU^R01^ORU_R01',ctrl),
       `PID|1||LABPAT00${i+1}^^^HOSP^MR||LABPT^TEST^${i+1}|||${i%2===0?'M':'F'}`,
       `PV1|1|I|LAB^L0${i+1}^A^HOSP`,...panels[i]].join(SEG)});
  }

  // ORU^R03 (3)
  for (let i=1;i<=3;i++) {
    const ctrl='ORUR03'+String(i).padStart(4,'0');
    corpus.push({type:'ORU^R03',label:`R03 #${i}`,message:
      [msh('LIS','ORU^R03^ORU_R01',ctrl),`PID|1||R3P00${i}^^^HOSP^MR||R03PT^TEST^${i}|||M`,
       `OBR|1|R3O00${i}|R3L00${i}|2093-3^CHOLESTEROL^LN|||${now}`,
       `OBX|1|NM|2093-3^CHOLESTEROL^LN||${155+i*20}|mg/dL|<200||||F|||${now}`].join(SEG)});
  }

  // ORM^O01 (3)
  const ords=[{c:'24323-8',n:'METABOLIC PANEL',p:'STAT'},{c:'71428-1',n:'CBC WITH DIFF',p:'R'},{c:'2085-9',n:'HDL CHOLESTEROL',p:'R'}];
  for (let i=0;i<3;i++) {
    const od=ords[i],ctrl='ORMO01'+String(i+1).padStart(4,'0');
    corpus.push({type:'ORM^O01',label:`Order ${od.n}`,message:
      [msh('EPIC','ORM^O01^ORM_O01',ctrl),`PID|1||OPAT00${i+1}^^^HOSP^MR||ORDERPT^TEST^${i+1}|||M`,
       `ORC|NW|ORD00${i+1}^EPIC||GRPID|SC||||${now}`,
       `OBR|1|ORD00${i+1}^EPIC|LAB00${i+1}|${od.c}^${od.n}^LN|||${now}|||||||${now}|SERUM|||${od.p}`].join(SEG)});
  }

  // MDM^T02 (3)
  const docs=[{c:'18842-5',n:'DISCHARGE SUMMARY',t:'Patient discharged in stable condition.'},{c:'34117-2',n:'H AND P',t:'Chest pain. BP 140/90. ECG ordered.'},{c:'11488-4',n:'CONSULT NOTE',t:'Cardiology: recommend echo.'}];
  for (let i=0;i<3;i++) {
    const dt=docs[i],ctrl='MDMT02'+String(i+1).padStart(4,'0');
    const b64=Buffer.from(dt.t).toString('base64');
    corpus.push({type:'MDM^T02',label:`${dt.n}`,message:
      [msh('EPIC','MDM^T02^MDM_T02',ctrl),`EVN|T02|${now}`,
       `PID|1||MPAT00${i+1}^^^HOSP^MR||DOCPT^TEST^${i+1}|||M`,
       `PV1|1|I|WARD01^10${i+1}^A^HOSP`,
       `TXA|1|${dt.c}^${dt.n}^LN|TX|${now}|DR00${i+1}^PROV^FIRST^^MD||||DOC00${i+1}|||AU|OA|AV`,
       `OBX|1|ED|${dt.c}^${dt.n}^LN||^TEXT^${dt.c}^Base64^${b64}||||||F`].join(SEG)});
  }

  // VXU^V04 (3)
  const vaxes=[{x:'140',n:'INFLUENZA',l:'LOT26A'},{x:'20',n:'DIPHTHERIA TETANUS',l:'LOT26B'},{x:'115',n:'TETANUS TOXOID',l:'LOT26C'}];
  for (let i=0;i<3;i++) {
    const v=vaxes[i],ctrl='VXUV04'+String(i+1).padStart(4,'0');
    corpus.push({type:'VXU^V04',label:`${v.n} vax`,message:
      [msh('IMMREG','VXU^V04^VXU_V04',ctrl),`PID|1||VPAT00${i+1}^^^CLINIC^MR||VAXPT^TEST^${i+1}|||${i%2===0?'F':'M'}`,
       `ORC|RE|VXU00${i+1}^CLINIC`,
       `RXA|0|1|${ts}|${ts}|${v.x}^${v.n}^CVX|1|mL|ML^^UCUM|00^NEW RECORD^NIP001|||CLINIC||${v.l}||||CP|A`,
       `RXR|C28161^IM^NCIT|LA^LEFT ARM^HL70163`].join(SEG)});
  }

  // SIU^S12 (3)
  const appts=['CONSULT','FOLLOWUP','PROCEDURE'];
  for (let i=1;i<=3;i++) {
    const ctrl='SIUS12'+String(i).padStart(4,'0');
    corpus.push({type:'SIU^S12',label:`${appts[i-1]} appt #${i}`,message:
      [msh('SCHED','SIU^S12^SIU_S12',ctrl),
       `SCH|APT00${i}^SCHED|||||||${appts[i-1]}|||30^MINUTES^ANS|||||DR00${i}^PROV^FIRST^^MD||APT00${i}`,
       `PID|1||SPAT00${i}^^^CLINIC^MR||APPTPT^TEST^${i}|||M`,
       `AIP|1||DR00${i}^PROV^FIRST^^MD^PHYSICIAN|||${ts}130000|30^MINUTES^ANS`].join(SEG)});
  }

  // DFT^P03 (3)
  const chgs=[{c:'99213',n:'OFFICE VISIT',a:'150.00'},{c:'36415',n:'BLOOD DRAW',a:'25.00'},{c:'93000',n:'ECG',a:'75.00'}];
  for (let i=0;i<3;i++) {
    const ch=chgs[i],ctrl='DFTP03'+String(i+1).padStart(4,'0');
    corpus.push({type:'DFT^P03',label:`${ch.n} charge`,message:
      [msh('BILLING','DFT^P03^DFT_P03',ctrl),`EVN|P03|${now}`,
       `PID|1||BPAT00${i+1}^^^HOSP^MR||BILLPT^TEST^${i+1}|||M`,
       `PV1|1|I|WARD01^10${i+1}^A^HOSP`,
       `FT1|1||TXN00${i+1}|${ts}||CG|${ch.c}^${ch.n}^CPT||||${ch.a}||DR00${i+1}^PROV^FIRST^^MD`,
       `DG1|1|ICD10|R07.9|CHEST PAIN|${now}|A`].join(SEG)});
  }

  // MFN^M13 (2)
  for (let i=1;i<=2;i++) {
    const ctrl='MFNM13'+String(i).padStart(4,'0');
    corpus.push({type:'MFN^M13',label:`Master file #${i}`,message:
      [msh('MASTERFILES','MFN^M13^MFN_M13',ctrl),
       `MFI|HL7${i*100}^SITE_DEFINED^L|${ctrl}|UPD|||AL`,
       `MFE|MAD|KEY00${i}|${now}|MFN_VALUE_${i}^LABEL${i}^L|CWE`].join(SEG)});
  }

  return corpus;
}


function sendMLLP(host, port, hl7Message, timeoutMs) {
  return new Promise((resolve) => {
    const SB = Buffer.from([0x0B]);
    const EB = Buffer.from([0x1C, 0x0D]);
    const msgBuf = Buffer.from(hl7Message, 'ascii');
    const packet = Buffer.concat([SB, msgBuf, EB]);
    const result = { success:false, ackCode:'', error:'' };
    const sock = new net.Socket();
    let ackBuf = Buffer.alloc(0), settled = false;
    const done = () => {
      if (settled) return; settled = true; clearTimeout(timer); sock.destroy();
      const raw = ackBuf.toString('ascii').replace(/^\x0B/, '').replace(/\x1C\x0D$/, '');
      const msa = raw.split(/\r|\n/).find(l => l.startsWith('MSA'));
      if (msa) { const p = msa.split('|'); result.ackCode = p[1] || ''; }
      result.success = true; resolve(result);
    };
    const timer = setTimeout(() => {
      if (!settled) { settled=true; result.error='Timeout'; sock.destroy(); resolve(result); }
    }, timeoutMs);
    sock.on('error', err => { if (!settled) { settled=true; result.error=err.message; clearTimeout(timer); sock.destroy(); resolve(result); } });
    sock.on('data', chunk => {
      ackBuf = Buffer.concat([ackBuf, chunk]);
      if (ackBuf.length >= 2 && ackBuf[ackBuf.length-1]===0x0D && ackBuf[ackBuf.length-2]===0x1C) done();
    });
    sock.on('close', done);
    sock.connect(port, host, () => { sock.write(packet); });
  });
}

function httpReq(urlStr, opts={}) {
  return new Promise((resolve, reject) => {
    const url = new URL(urlStr);
    const lib = url.protocol === 'https:' ? https : http;
    const options = {
      hostname: url.hostname, port: url.port||(url.protocol==='https:'?443:80),
      path: url.pathname+url.search, method: opts.method||'GET',
      headers: { 'Content-Type':'application/json', ...(opts.headers||{}) },
    };
    const req = lib.request(options, res => {
      const chunks = []; res.on('data', c => chunks.push(c));
      res.on('end', () => {
        const body = Buffer.concat(chunks).toString('utf8');
        try { resolve({status:res.statusCode, body:JSON.parse(body), headers:res.headers}); }
        catch { resolve({status:res.statusCode, body, headers:res.headers}); }
      });
    });
    req.on('error', reject);
    if (opts.body) req.write(JSON.stringify(opts.body));
    req.end();
  });
}

async function login(base, email, password) {
  const r = await httpReq(`${base}/api/auth/login`, {method:'POST', body:{email,password}});
  if (!r.body.token) throw new Error('Login failed: '+JSON.stringify(r.body));
  // Capture session cookie for subsequent session-authenticated requests
  const setCookies = r.headers['set-cookie'] || [];
  const cookie = setCookies.map(c => c.split(';')[0]).join('; ');
  return { token: r.body.token, cookie };
}

async function getMessages(base, auth, ifaceId, limit=200, page=1) {
  const cookieHdr = typeof auth === 'string' ? {} : {Cookie: auth.cookie};
  return (await httpReq(`${base}/api/messages/interface/${ifaceId}?limit=${limit}&page=${page}`,
    {headers: cookieHdr})).body;
}

async function getFHIR(goBase, msgId, ifaceId) {
  try {
    const r = await httpReq(`${goBase}/api/messages/${msgId}/raw?interfaceId=${ifaceId}`);
    if (r.body.success && r.body.content)
      return typeof r.body.content==='string' ? JSON.parse(r.body.content) : r.body.content;
  } catch {}
  return null;
}

function validate(bundle, msgType, level='standard', rawHL7='') {
  const issues = [];
  if (!bundle) { issues.push('NULL bundle'); return {pass:false, issues, resourceTypes:[]}; }
  if (bundle.resourceType !== 'Bundle') issues.push(`resourceType='${bundle.resourceType}' expected Bundle`);
  if (!bundle.entry||bundle.entry.length===0) { issues.push('entry is empty'); return {pass:false,issues,resourceTypes:[]}; }
  const rts = bundle.entry.map(e => e.resource ? e.resource.resourceType : '(none)');
  if (rts[0] !== 'MessageHeader') issues.push(`First entry '${rts[0]}' not MessageHeader`);
  if (level==='basic') return {pass:issues.length===0, issues, resourceTypes:rts};
  (REQUIRED[msgType]||[]).forEach(req => { if (!rts.includes(req)) issues.push(`Missing: ${req}`); });
  const pat = bundle.entry.find(e=>e.resource&&e.resource.resourceType==='Patient');
  if (pat) {
    const p=pat.resource;
    if (!((p.identifier&&p.identifier.length>0)||(p.name&&p.name.length>0))) issues.push('Patient missing identifier and name');
  }
  const bad=bundle.entry.filter(e=>!e.resource||!e.resource.resourceType);
  if (bad.length) issues.push(`${bad.length} entries missing resourceType`);
  if (level==='strict') {
    const nc={
      MessageHeader:['eventCoding'],
      Encounter:['status','class'],
      Observation:['status','code'],
      DiagnosticReport:['status','code'],
      Immunization:['status','vaccineCode'],
      Appointment:['status','participant'],
      ServiceRequest:['status','intent'],
      DocumentReference:['status','content'],
    };
    bundle.entry.forEach(e => {
      if (!e.resource) return; const rt=e.resource.resourceType;
      (nc[rt]||[]).forEach(f => { if (e.resource[f]==null) issues.push(`${rt}.${f} null`); });
    });
  }
  if (level==='field') {
    fieldLevelChecks(rawHL7, bundle, msgType).forEach(i => issues.push(i));
  }
  return {pass:issues.length===0, issues, resourceTypes:rts};
}


// ── Field-level semantic validation helpers (--level field) ─────────────────

function hl7Segs(raw, name) {
  return raw.split(/\r/).filter(s => s.startsWith(name + '|'));
}
function hl7Field(seg, fieldIdx, compIdx) {
  const f = (seg.split('|')[fieldIdx] || '');
  return compIdx == null ? f : (f.split('^')[compIdx - 1] || '');
}
function fhirRes(bundle, rt) {
  const e = (bundle.entry || []).find(x => x.resource && x.resource.resourceType === rt);
  return e ? e.resource : null;
}
function fhirAll(bundle, rt) {
  return (bundle.entry || []).filter(x => x.resource && x.resource.resourceType === rt).map(x => x.resource);
}

function fieldLevelChecks(rawHL7, bundle, msgType) {
  const issues = [];
  const base = msgType.split('^')[0]; // ADT, ORU, ORM, MDM, VXU, SIU, DFT, MFN
  const event = msgType.split('^')[1] || ''; // A01, R01, S12, …

  // ── MessageHeader: MSH.9.1 → event code ──────────────────────────────────
  const msh = hl7Segs(rawHL7, 'MSH')[0] || '';
  if (msh) {
    const msgTypeField = hl7Field(msh, 9, 1); // e.g. "ADT" in MSH.9.1
    const mhRes = fhirRes(bundle, 'MessageHeader');
    if (mhRes && msgTypeField) {
      const evCode = (mhRes.eventCoding && mhRes.eventCoding.code) ||
                     (mhRes.event && (typeof mhRes.event === 'string' ? mhRes.event : mhRes.event.code)) || '';
      // Accept either the full type (e.g. "A01") or message class (e.g. "ADT")
      if (evCode && !evCode.includes(msgTypeField) && !evCode.includes(event)) {
        issues.push(`MessageHeader.eventCoding.code='${evCode}' — expected to reference MSH.9='${msgTypeField}'`);
      }
    }
  }

  // ── Patient: PID.5.1 → name[0].family ; PID.3.1 → identifier[0].value ────
  const pid = hl7Segs(rawHL7, 'PID')[0] || '';
  if (pid) {
    const expectedFamily = hl7Field(pid, 5, 1);
    const expectedMRN    = hl7Field(pid, 3, 1);
    const patRes = fhirRes(bundle, 'Patient');
    if (patRes) {
      if (expectedFamily) {
        const actualFamily = (patRes.name && patRes.name[0] && patRes.name[0].family) || '';
        if (!actualFamily.toLowerCase().includes(expectedFamily.toLowerCase())) {
          issues.push(`Patient.name[0].family='${actualFamily}' — expected to contain PID.5.1='${expectedFamily}'`);
        }
      }
      if (expectedMRN) {
        const ids = patRes.identifier || [];
        const hasId = ids.some(id => id.value && id.value.includes(expectedMRN));
        if (!hasId) issues.push(`Patient.identifier missing PID.3.1='${expectedMRN}'`);
      }

      // PID.7 → Patient.birthDate (HL7 YYYYMMDD → FHIR YYYY-MM-DD)
      const dob = hl7Field(pid, 7).substring(0, 8); // take date portion only
      if (dob && dob.length >= 8) {
        const expected = `${dob.substring(0,4)}-${dob.substring(4,6)}-${dob.substring(6,8)}`;
        if (patRes.birthDate && patRes.birthDate !== expected) {
          issues.push(`Patient.birthDate='${patRes.birthDate}' — expected PID.7='${expected}'`);
        }
      }

      // PID.8 → Patient.gender (M→male, F→female, U→unknown, O→other)
      const sex = hl7Field(pid, 8).toUpperCase();
      const genderMap = { M: 'male', F: 'female', U: 'unknown', O: 'other' };
      if (sex && genderMap[sex]) {
        if (patRes.gender && patRes.gender !== genderMap[sex]) {
          issues.push(`Patient.gender='${patRes.gender}' — expected PID.8='${sex}'→'${genderMap[sex]}'`);
        }
      }
    }
  }

  // ── Encounter: PV1.2 → class.code ────────────────────────────────────────
  if (['ADT','DFT'].includes(base)) {
    const pv1 = hl7Segs(rawHL7, 'PV1')[0] || '';
    if (pv1) {
      const expectedClass = hl7Field(pv1, 2);
      const encRes = fhirRes(bundle, 'Encounter');
      if (encRes && expectedClass) {
        const actualClass = (encRes.class && (encRes.class.code || encRes.class)) || '';
        if (String(actualClass).toUpperCase() !== expectedClass.toUpperCase()) {
          issues.push(`Encounter.class.code='${actualClass}' — expected PV1.2='${expectedClass}'`);
        }
      }
    }
  }

  // ── Observation: OBX.3.1 → code.coding[0].code ; OBX.5 → value ──────────
  if (['ORU'].includes(base)) {
    const obxSegs = hl7Segs(rawHL7, 'OBX');
    const obsArr  = fhirAll(bundle, 'Observation');
    obxSegs.forEach((obx, idx) => {
      const loincCode  = hl7Field(obx, 3, 1);
      const rawValue   = hl7Field(obx, 5, 1) || hl7Field(obx, 5);
      const obs = obsArr[idx];
      if (!obs) return;
      if (loincCode) {
        const codings = (obs.code && obs.code.coding) || [];
        const found   = codings.some(c => c.code === loincCode);
        if (!found) issues.push(`Observation[${idx}].code.coding missing OBX[${idx}].3.1='${loincCode}'`);
      }
      if (rawValue && rawValue !== '') {
        const hasValue = obs.valueQuantity != null || obs.valueString != null ||
                         obs.valueCodeableConcept != null || obs.valueBoolean != null;
        if (!hasValue) issues.push(`Observation[${idx}] has no value — OBX[${idx}].5='${rawValue}'`);
      }
    });
  }

  // ── Immunization: RXA.5.1 (CVX) → vaccineCode.coding[0].code ────────────
  if (base === 'VXU') {
    const rxa = hl7Segs(rawHL7, 'RXA')[0] || '';
    if (rxa) {
      const cvxCode = hl7Field(rxa, 5, 1);
      const immRes  = fhirRes(bundle, 'Immunization');
      if (immRes && cvxCode) {
        const codings = (immRes.vaccineCode && immRes.vaccineCode.coding) || [];
        const found   = codings.some(c => c.code === cvxCode);
        if (!found) issues.push(`Immunization.vaccineCode.coding missing RXA.5.1='${cvxCode}'`);
      }
    }
  }

  // ── DocumentReference: TXA.2.1 → type.coding[0].code ────────────────────
  if (base === 'MDM') {
    const txa = hl7Segs(rawHL7, 'TXA')[0] || '';
    if (txa) {
      const docType = hl7Field(txa, 2, 1);
      const drRes   = fhirRes(bundle, 'DocumentReference');
      if (drRes && docType) {
        const codings = (drRes.type && drRes.type.coding) || [];
        const found   = codings.some(c => c.code === docType);
        if (!found) issues.push(`DocumentReference.type.coding missing TXA.2.1='${docType}'`);
      }
    }
  }

  // ── ServiceRequest: OBR.4.1 → code.coding[0].code ───────────────────────
  if (base === 'ORM') {
    const obr = hl7Segs(rawHL7, 'OBR')[0] || '';
    if (obr) {
      const orderCode = hl7Field(obr, 4, 1);
      const srRes     = fhirRes(bundle, 'ServiceRequest');
      if (srRes && orderCode) {
        const codings = (srRes.code && srRes.code.coding) || [];
        const found   = codings.some(c => c.code === orderCode);
        if (!found) issues.push(`ServiceRequest.code.coding missing OBR.4.1='${orderCode}'`);
      }
    }
  }

  // ── Appointment: SCH.7 → serviceType/appointmentType ; SCH.11 → duration ─
  if (base === 'SIU') {
    const sch = hl7Segs(rawHL7, 'SCH')[0] || '';
    if (sch) {
      const apptTypeText = hl7Field(sch, 7, 1) || hl7Field(sch, 7); // free-text type
      const apptRes      = fhirRes(bundle, 'Appointment');
      if (apptRes) {
        if (apptTypeText) {
          // Check either serviceType or appointmentType carries the scheduled type
          const stCodings = ((apptRes.serviceType || [])[0] || {}).coding || [];
          const atCodings = ((apptRes.appointmentType || {}).coding) || [];
          const allCodings = [...stCodings, ...atCodings];
          const allTexts = [
            ...stCodings.map(c => (c.display||'').toLowerCase()),
            ...atCodings.map(c => (c.display||'').toLowerCase()),
            ((apptRes.serviceType || [])[0] || {}).text || '',
            ((apptRes.appointmentType || {}).text) || '',
          ].filter(Boolean);
          const typeMatch = allCodings.some(c => c.code === apptTypeText) ||
                            allTexts.some(t => t.toLowerCase().includes(apptTypeText.toLowerCase()));
          if (!typeMatch) {
            issues.push(`Appointment serviceType/appointmentType missing SCH.7='${apptTypeText}'`);
          }
        }

        // SCH.11.1 = duration in minutes → Appointment.minutesDuration
        const durStr = hl7Field(sch, 11, 1);
        if (durStr && !isNaN(parseInt(durStr, 10))) {
          const expectedMins = parseInt(durStr, 10);
          if (apptRes.minutesDuration != null && apptRes.minutesDuration !== expectedMins) {
            issues.push(`Appointment.minutesDuration=${apptRes.minutesDuration} — expected SCH.11.1=${expectedMins}`);
          }
        }
      }
    }
  }

  return issues;
}


function writeReport(results, outPath, summary) {
  const passed = results.filter(r=>r.fhirValid).length;
  const pct = Math.round(passed/results.length*1000)/10;
  const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  const rows = results.map(r => {
    const sc=r.fhirValid?'pass':'fail', st=r.fhirValid?'PASS':'FAIL';
    const ac=r.ackCode==='AA'?'pass':'fail';
    const iss=r.issues.length?r.issues.map(esc).join('<br>'):'-';
    const tps=r.resourceTypes.length?r.resourceTypes.map(esc).join(', '):'-';
    let hl7cell = '-';
    if (!r.fhirValid && r.hl7) {
      const hl7text = esc(r.hl7.split(/\r/).join('\n'));
      hl7cell = `<details><summary style="cursor:pointer;color:#2563eb;font-size:11px">show HL7</summary><pre class="hl7pre">${hl7text}</pre></details>`;
    }
    return `<tr><td><span class="badge ${sc}">${st}</span></td><td>${esc(r.messageType)}</td><td>${esc(r.label)}</td><td><span class="badge ${ac}">${esc(r.ackCode||'-')}</span></td><td class="mono">${esc(r.messageId||'-')}</td><td class="small">${tps}</td><td class="issues">${iss}</td><td>${hl7cell}</td></tr>`;
  }).join('\n');
  const html = `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>HL7 FHIR Scale Test</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;margin:0;background:#f5f7fa}header{background:#1a3a5c;color:#fff;padding:24px 32px}header h1{margin:0 0 4px;font-size:22px}header p{margin:0;opacity:.75;font-size:13px}.summary{display:flex;gap:16px;padding:20px 32px;background:#fff;border-bottom:1px solid #e0e4ea;flex-wrap:wrap}.card{flex:1;min-width:120px;background:#f9fafb;border:1px solid #e0e4ea;border-radius:8px;padding:16px 20px}.card .num{font-size:32px;font-weight:700}.card .lbl{font-size:12px;text-transform:uppercase;letter-spacing:.05em;color:#888;margin-top:2px}.card.green .num{color:#16a34a}.card.red .num{color:#dc2626}.card.blue .num{color:#2563eb}.card.gray .num{color:#6b7280}table{width:100%;border-collapse:collapse;font-size:13px}th{background:#1a3a5c;color:#fff;padding:10px 12px;text-align:left;font-weight:600}td{padding:9px 12px;border-bottom:1px solid #e8eaed;vertical-align:top}tr:hover td{background:#f0f4ff}.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:700}.pass{background:#dcfce7;color:#15803d}.fail{background:#fee2e2;color:#b91c1c}.small{font-size:11px;color:#555;max-width:320px;word-break:break-all}.issues{font-size:11px;color:#b91c1c;max-width:260px}.mono{font-family:monospace;font-size:11px}.wrap{padding:0 32px 32px}.bar{height:8px;background:#e5e7eb;border-radius:4px;margin:8px 0}.fill{height:100%;border-radius:4px;background:#16a34a}.hl7pre{background:#0f172a;color:#e2e8f0;font-family:monospace;font-size:11px;padding:10px 12px;border-radius:6px;white-space:pre-wrap;word-break:break-all;margin:6px 0 2px;max-width:520px;max-height:260px;overflow:auto}</style></head>
<body><header><h1>HL7 to FHIR Scale Test Report</h1><p>Generated ${new Date().toISOString()} | Validation: ${summary.level}</p></header>
<div class="summary">
<div class="card blue"><div class="num">${summary.sent}</div><div class="lbl">Messages sent</div></div>
<div class="card gray"><div class="num">${summary.acked}</div><div class="lbl">ACK AA</div></div>
<div class="card gray"><div class="num">${summary.retrieved}</div><div class="lbl">Bundles retrieved</div></div>
<div class="card green"><div class="num">${summary.passed}</div><div class="lbl">Passed</div></div>
<div class="card red"><div class="num">${summary.failed}</div><div class="lbl">Failed</div></div>
</div><div class="wrap">
<div style="margin:16px 0 4px;font-weight:600">Pass rate: ${pct}%</div>
<div class="bar"><div class="fill" style="width:${pct}%"></div></div>
<table><thead><tr><th>Result</th><th>Type</th><th>Variant</th><th>ACK</th><th>FHIR Message ID</th><th>Resources</th><th>Issues</th><th>Input HL7</th></tr></thead><tbody>${rows}</tbody></table></div></body></html>`;
  fs.writeFileSync(outPath, html, 'utf8');
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function main() {
  console.log('\n=== HL7 to FHIR Scale Test ===');
  fs.mkdirSync(CONFIG.resultsDir, {recursive:true});

  let corpus = buildCorpus();
  if (FILTER_TYPES.length) corpus = corpus.filter(m => FILTER_TYPES.includes(m.type));
  if (REPEAT_EACH > 1) { const ex=[]; corpus.forEach(m => { for (let r=1;r<=REPEAT_EACH;r++) ex.push({...m,label:`${m.label} [r${r}]`}); }); corpus=ex; }
  const typeCount = [...new Set(corpus.map(m=>m.type))].length;
  console.log(`[1/5] Corpus: ${corpus.length} messages, ${typeCount} types`);
  const byType={}; corpus.forEach(m=>{byType[m.type]=(byType[m.type]||0)+1;});
  Object.entries(byType).forEach(([t,c])=>console.log(`      ${t}: ${c}`));

  console.log('\n[2/5] Authenticating...');
  const auth = await login(CONFIG.nodeBase, CONFIG.adminEmail, CONFIG.adminPassword);
  console.log(`     OK (cookie: ${auth.cookie.substring(0,40)}...)`);

  // Snapshot output count BEFORE sending — pipeline is fast so capture now
  let countBefore = 0;
  try {
    const snap = await getMessages(CONFIG.nodeBase, auth, CONFIG.outputInterfaceId, 1, 1);
    if (snap.success && snap.data && snap.data.pagination) countBefore = snap.data.pagination.totalCount;
    else console.log(`  (baseline snap): ${JSON.stringify(snap).substring(0,120)}`);
  } catch (e) { console.log(`  (baseline err): ${e.message}`); }
  console.log(`  Output baseline count (before sends): ${countBefore}`);

  console.log(`\n[3/5] Sending via MLLP to port ${CONFIG.mlllpPort} (concurrent=${CONCURRENT})...`);
  const startTime = new Date();
  const sendResults = [];
  let ackCount = 0;

  // Split corpus into batches of CONCURRENT, send each batch in parallel
  for (let bStart = 0; bStart < corpus.length; bStart += CONCURRENT) {
    const batch = corpus.slice(bStart, bStart + CONCURRENT);
    const bResults = await Promise.all(batch.map(msg =>
      sendMLLP(CONFIG.mlllpHost, CONFIG.mlllpPort, msg.message, CONFIG.socketTimeoutMs)
        .then(r => ({ msg, r }))
    ));
    for (const { msg, r } of bResults) {
      sendResults.push({type:msg.type,label:msg.label,ackCode:r.ackCode,sendOK:r.success,sendErr:r.error,hl7:msg.message});
      const icon = r.ackCode==='AA'?'[AA]':(r.success?`[${r.ackCode}]`:'[ERR]');
      console.log(`  ${icon} ${msg.type.padEnd(12)} ${msg.label}${r.error?' ERROR:'+r.error:''}`);
      if (r.ackCode==='AA') ackCount++;
    }
    if (CONCURRENT > 1 && bStart + CONCURRENT < corpus.length) await sleep(100);
  }
  const throughputSec = (corpus.length / ((Date.now() - startTime.getTime()) / 1000)).toFixed(1);
  console.log(`  Sent: ${corpus.length}  ACK AA: ${ackCount}  Throughput: ${throughputSec} msg/s`);

  console.log(`\n[4/5] Waiting ${WAIT_SECS}s then polling output...`);
  await sleep(WAIT_SECS * 1000);

  // Poll until we see at least ackCount new messages (or 90s deadline)
  const allOutput = [];
  const deadline = Date.now() + 90000;
  while (Date.now() < deadline) {
    const snap2 = await getMessages(CONFIG.nodeBase, auth, CONFIG.outputInterfaceId, 1, 1);
    if (!snap2.success) { console.log(`  poll not-ok: ${JSON.stringify(snap2).substring(0,100)}`); await sleep(2000); continue; }
    const totalNow = snap2.data && snap2.data.pagination ? snap2.data.pagination.totalCount : 0;
    const newCount = totalNow - countBefore;
    console.log(`  poll: total=${totalNow} new=${newCount} need=${ackCount}`);
    if (newCount >= ackCount || (newCount > 0 && Date.now() + 5000 > deadline)) {
      // Fetch exactly the new messages (they're first in DESC order)
      const fetchN = Math.max(newCount, ackCount);
      const pages = Math.ceil(fetchN / 200);
      for (let p = 1; p <= pages; p++) {
        const r = await getMessages(CONFIG.nodeBase, auth, CONFIG.outputInterfaceId, 200, p);
        if (!r.success || !r.data || !r.data.messages) break;
        allOutput.push(...r.data.messages.slice(0, fetchN - allOutput.length));
        if (allOutput.length >= fetchN) break;
      }
      break;
    }
    await sleep(2000);
  }
  // DESC -> ASC to match send order
  allOutput.sort((a,b) => new Date(a.received_at)-new Date(b.received_at));
  console.log(`  Found ${allOutput.length} FHIR bundle(s)`);

  console.log('\n[5/5] Validating...');
  const stamp = new Date().toISOString().replace(/[:.]/g,'-').substring(0,19);
  const failDir = path.join(CONFIG.resultsDir, `failing_${stamp}`);
  const results = [];
  let failSaved = 0;
  for (let k=0; k<sendResults.length; k++) {
    const sent=sendResults[k], outMsg=k<allOutput.length?allOutput[k]:null;
    let msgId='', bundle=null, fhirValid=false, issues=[], resourceTypes=[];
    if (outMsg) {
      msgId=outMsg.message_id;
      bundle=await getFHIR(CONFIG.goBase, msgId, CONFIG.outputInterfaceId);
      const v=validate(bundle, sent.type, VALID_LEVEL, sent.hl7||'');
      fhirValid=v.pass; issues=v.issues; resourceTypes=v.resourceTypes;
    } else { issues=['No output message found']; }
    results.push({messageType:sent.type,label:sent.label,ackCode:sent.ackCode,sendOK:sent.sendOK,messageId:msgId,fhirValid,issues,resourceTypes,hl7:sent.hl7});
    const icon=fhirValid?'[PASS]':'[FAIL]', rts=resourceTypes.join(', ')||'-';
    console.log(`\n${'─'.repeat(72)}`);
    console.log(`  ${icon} ${sent.type.padEnd(12)} ${sent.label}`);
    console.log(`  Resources : ${rts}`);
    if (issues.length) issues.forEach(iss => console.log(`  Issue     : ${iss}`));
    // ── Input HL7 ──
    console.log('\n  ── Input HL7 ──');
    sent.hl7.split(/\r/).filter(Boolean).forEach(seg => console.log(`  ${seg}`));
    // ── Output FHIR ──
    console.log('\n  ── Output FHIR ──');
    if (bundle) {
      console.log(JSON.stringify(bundle, null, 2).split('\n').map(l=>'  '+l).join('\n'));
    } else {
      console.log('  (no bundle retrieved)');
    }
    // Save artefacts for every failing message
    if (!fhirValid) {
      if (failSaved === 0) fs.mkdirSync(failDir, {recursive:true});
      const slug = `${sent.type.replace(/[\^]/g,'_')}_${sent.label.replace(/[^a-zA-Z0-9]+/g,'_').replace(/^_|_$/g,'')}`;
      // HL7: replace \r with real newlines for readability
      const hl7Lines = sent.hl7.split(/\r/).join('\n');
      fs.writeFileSync(path.join(failDir, `${slug}.hl7`), hl7Lines, 'utf8');
      if (bundle) {
        fs.writeFileSync(path.join(failDir, `${slug}.fhir.json`), JSON.stringify(bundle, null, 2), 'utf8');
      } else {
        fs.writeFileSync(path.join(failDir, `${slug}.fhir.json`), JSON.stringify({error:'no bundle retrieved', messageId:msgId}, null, 2), 'utf8');
      }
      failSaved++;
    }
  }

  const passed=results.filter(r=>r.fhirValid).length, failed=results.length-passed;
  const pct=(passed/results.length*100).toFixed(1);
  console.log(`\n=== RESULTS: Sent ${corpus.length}  ACK ${ackCount}  Bundles ${allOutput.length}  Pass ${passed}  Fail ${failed}  Rate ${pct}% ===`);
  if (failed) {
    console.log(`\nFailed artefacts saved to: ${failDir}`);
    results.filter(r=>!r.fhirValid).forEach(r=>{console.log(`  ${r.messageType} - ${r.label}`);r.issues.forEach(i=>console.log(`    >> ${i}`));});
  }

  const durationMs = Date.now() - startTime.getTime();
  const summary={sent:corpus.length,acked:ackCount,retrieved:allOutput.length,passed,failed,level:VALID_LEVEL,concurrent:CONCURRENT,durationMs,inputInterfaceId:CONFIG.inputInterfaceId};
  fs.writeFileSync(path.join(CONFIG.resultsDir,`results_${stamp}.json`), JSON.stringify({summary,results},null,2), 'utf8');
  writeReport(results, path.join(CONFIG.resultsDir,`report_${stamp}.html`), summary);
  console.log(`\nReport: ${path.join(CONFIG.resultsDir,'report_'+stamp+'.html')}`);
}

main().catch(err => { console.error('Fatal:', err); process.exit(1); });
