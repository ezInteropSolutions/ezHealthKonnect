# Make.com Daily Content Generation — Prompt Templates

## Overview of the Make Flow

```
[Schedule: Daily 8am] 
    → [HTTP: POST to Gemini API]
        → [Parse response]
            → [Email/Slack to Shanawaz for review]
                → [Approval step]
                    → [Post to LinkedIn / publish blog draft]
```

---

## Module 1: Gemini System Prompt (paste into the "System" field)

Copy this into the Gemini API module's system/context field:

```
You are a content strategist and writer for ezHealthKonnect, a healthcare integration platform. 
Your job is to generate one daily content idea and draft it in two formats: 
a LinkedIn post and a blog article outline.

[PASTE FULL CONTENTS OF gemini_brand_context.md HERE]
```

---

## Module 2: Daily User Prompt (paste into the "User message" field)

This is the prompt Make sends each day. Use the `{{date}}` variable for today's date.

```
Today is {{date}}.

Your task: Generate THREE different LinkedIn posts for ezHealthKonnect for today.
Each post must use a DIFFERENT content pillar and target a DIFFERENT audience angle.

Content Pillars (do not repeat a pillar across the 3 posts today):
1. Healthcare Interoperability 101 (education — HL7, FHIR, CDA concepts)
2. Feature Spotlight (one specific ezHealthKonnect capability)
3. Use Case / Integration Scenario (real-world problem + solution story)
4. Pain Point + Solution (a specific integration headache, then the fix)
5. Industry News + Perspective (CMS, ONC, TEFCA, or interoperability mandates)
6. Technical Deep Dive (for engineers — MLLP, HL7 anatomy, FHIR bundles)
7. ROI / Business Case (time saved, cost comparison, compliance value)

Post format guidance:
- Post A: 150-200 words, broad healthcare IT audience (CIOs, IT Directors)
- Post B: 200-250 words, technical/engineer audience (Integration Engineers, Interface Analysts)
- Post C: 100-150 words, punchy/conversational — hook-driven, shareable

Respond in the following JSON format — nothing else, no extra text:

{
  "date": "{{date}}",
  "posts": [
    {
      "id": "A",
      "pillar": "<pillar number and name>",
      "topic": "<specific topic title>",
      "audience": "executive",
      "hook": "<one sentence that immediately grabs a healthcare IT leader's attention>",
      "draft": "<complete LinkedIn post, using line breaks for readability, ending with 2-3 relevant hashtags like #HealthcareIT #HL7 #FHIR #Interoperability>",
      "suggested_image": "<description of an image or graphic that would complement this post>",
      "approval_notes": "<reviewer tip — e.g., 'timely given the new CMS rule', 'add a screenshot of the pipeline builder'>"
    },
    {
      "id": "B",
      "pillar": "<pillar number and name>",
      "topic": "<specific topic title>",
      "audience": "engineer",
      "hook": "<one sentence that grabs an integration engineer's attention>",
      "draft": "<complete LinkedIn post, using line breaks for readability, ending with hashtags>",
      "suggested_image": "<description of complementary image>",
      "approval_notes": "<reviewer tip>"
    },
    {
      "id": "C",
      "pillar": "<pillar number and name>",
      "topic": "<specific topic title>",
      "audience": "general",
      "hook": "<punchy one-liner hook>",
      "draft": "<short punchy LinkedIn post, conversational tone, highly shareable, ending with hashtags>",
      "suggested_image": "<description of complementary image>",
      "approval_notes": "<reviewer tip>"
    }
  ]
}
```

---

## Module 3: Email Notification to Reviewer

Use Make's **Gmail** or **Email** module. After parsing the Gemini JSON, iterate over the 3 posts and send a single email.

**Subject:** `ezHealthKonnect Posts for Review — {{date}}`

**Body (HTML email):**
```
📋 3 POSTS FOR YOUR REVIEW — {{date}}

Pick the ones you want to post today. Click the approve link under each one.

━━━━━━━━━━━━━━━━━━━━━━━
POST A — {{posts[0].audience}} audience
Pillar: {{posts[0].pillar}} | Topic: {{posts[0].topic}}
━━━━━━━━━━━━━━━━━━━━━━━

{{posts[0].draft}}

🖼  Image idea: {{posts[0].suggested_image}}
💡 Notes: {{posts[0].approval_notes}}

✅ APPROVE POST A → {{webhook_base_url}}?post=A&date={{date}}

━━━━━━━━━━━━━━━━━━━━━━━
POST B — {{posts[1].audience}} audience
Pillar: {{posts[1].pillar}} | Topic: {{posts[1].topic}}
━━━━━━━━━━━━━━━━━━━━━━━

{{posts[1].draft}}

🖼  Image idea: {{posts[1].suggested_image}}
💡 Notes: {{posts[1].approval_notes}}

✅ APPROVE POST B → {{webhook_base_url}}?post=B&date={{date}}

━━━━━━━━━━━━━━━━━━━━━━━
POST C — {{posts[2].audience}} audience
Pillar: {{posts[2].pillar}} | Topic: {{posts[2].topic}}
━━━━━━━━━━━━━━━━━━━━━━━

{{posts[2].draft}}

🖼  Image idea: {{posts[2].suggested_image}}
💡 Notes: {{posts[2].approval_notes}}

✅ APPROVE POST C → {{webhook_base_url}}?post=C&date={{date}}

━━━━━━━━━━━━━━━━━━━━━━━
📝 Request revision: Reply to this email with feedback on any post.
```

---

## Module 4: Approval Webhook → LinkedIn Company Page Post

Create **one** Make webhook that receives `?post=A|B|C&date=...`.

**Webhook scenario steps:**
1. **Webhook trigger** — catches the approval click
2. **Data Store lookup** — retrieve today's 3 drafts by date key (stored in step after Gemini call)
3. **Switch/Router** — pick post A, B, or C based on `post` param
4. **LinkedIn module** → `Create a company update`
   - Organization ID: your ezHealthKonnect LinkedIn company page ID
   - Text: the approved post draft
   - Visibility: PUBLIC
5. **Email** → send you a confirmation + the approved post text ready to paste into groups

**LinkedIn App Setup** (one-time):
1. Go to [linkedin.com/developers](https://www.linkedin.com/developers/) → Create App
2. Associate it with the ezHealthKonnect company page
3. Request product: **Marketing Developer Platform** (needed for company page posting)
4. Scopes needed: `w_organization_social`, `r_organization_social`
5. Get your Organization ID: go to your company page → the URL contains it (`/company/12345678/`)

---

## LinkedIn Groups — Manual Step (API Limitation)

**LinkedIn shut down third-party group posting API in 2018.** No tool (Make, Zapier, Buffer, Hootsuite) can post to groups automatically. This is a LinkedIn policy restriction, not a Make limitation.

**Best workflow for groups:**

The approval confirmation email (step 5 above) sends you this:

```
✅ Posted to ezHealthKonnect Company Page!

━━━━━━━━━━━━━━━━━━━━━━━
READY TO PASTE INTO GROUPS
━━━━━━━━━━━━━━━━━━━━━━━
[approved post text here]

Groups to post in:
→ HL7 International: linkedin.com/groups/37013
→ FHIR Implementers: linkedin.com/groups/6811259
→ Healthcare Interoperability: linkedin.com/groups/3044917
→ Health IT Professionals: linkedin.com/groups/51731

Open each link and paste the text above. ~2 minutes total.
```

This is a 2-minute manual step. There is no compliant workaround.

---

## Gemini API Settings (Google AI Studio / Vertex AI)

| Setting | Value |
|---|---|
| Model | `gemini-1.5-pro` or `gemini-2.0-flash` |
| Temperature | `0.8` (creative but not random) |
| Max output tokens | `2048` |
| Response format | JSON (enable JSON mode if available) |
| Top-P | `0.95` |

---

## Make Scenario 1: Daily Generation (runs at 8am)

| # | Module | Details |
|---|---|---|
| 1 | **Schedule** | Every day 8:00 AM your timezone |
| 2 | **HTTP (Gemini API)** | POST to `https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key={{GEMINI_API_KEY}}` |
| 3 | **JSON Parse** | Parse `candidates[0].content.parts[0].text` as JSON |
| 4 | **Data Store: Set** | Store the 3 posts keyed by today's date (needed for webhook retrieval) |
| 5 | **Gmail / Email** | Send review email to shanawaz107@gmail.com |

## Make Scenario 2: Approval Webhook (triggered by email click)

| # | Module | Details |
|---|---|---|
| 1 | **Webhook** | Custom webhook URL (copy this as `{{webhook_base_url}}` in email template) |
| 2 | **Data Store: Get** | Retrieve today's posts by date from `?date=` param |
| 3 | **Switch** | Route on `?post=A`, `B`, or `C` |
| 4 | **LinkedIn: Create post** | Post to company page — organization URN: `urn:li:organization:YOUR_ORG_ID` |
| 5 | **Gmail** | Send confirmation + group-posting copy-paste email |

---

## Gemini API Call (HTTP Module Config)

**URL:** `https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key={{GEMINI_API_KEY}}`

**Method:** POST

**Headers:** `Content-Type: application/json`

**Body:**
```json
{
  "system_instruction": {
    "parts": [{ "text": "<PASTE gemini_brand_context.md CONTENTS HERE>" }]
  },
  "contents": [{
    "parts": [{ "text": "<PASTE Module 2 prompt here with {{date}} resolved>" }]
  }],
  "generationConfig": {
    "temperature": 0.8,
    "topP": 0.95,
    "maxOutputTokens": 3000,
    "responseMimeType": "application/json"
  }
}
```

---

## Tips for the Make Build

- **Store brand context** in a Make Data Store (key: `brand_context`) and fetch it in step 2 — update the context without rebuilding the scenario
- **Track used pillars** in a Data Store (`{week: "2026-W24", used_pillars: [1, 3, 5]}`) — include them in the prompt so Gemini avoids same-week repeats
- **Data Store for drafts** — key by date string (e.g., `"2026-06-14"`) so the webhook can look up the exact drafts without re-calling Gemini
- **Google Sheets log** — append each approved post to a sheet with date, pillar, audience, post text — content calendar built automatically
- **Revision path** — if you reply to the email, Make's Gmail watch module catches it, feeds your feedback + original draft back to Gemini for a refined version, re-sends for approval

---

## LinkedIn Groups to Target

Post manually into these groups after company page approval:

| Group | LinkedIn URL |
|---|---|
| HL7 International | linkedin.com/groups/37013 |
| FHIR Implementers Forum | linkedin.com/groups/6811259 |
| Healthcare Interoperability | linkedin.com/groups/3044917 |
| Health IT Professionals | linkedin.com/groups/51731 |
| Clinical Informatics Network | linkedin.com/groups/2290549 |
| Epic Users Group | linkedin.com/groups/149876 |

Verify group names/URLs before saving — LinkedIn group URLs can change.
