# ezIdentitySense Bulk Export to Files

## What this does

Every day at a time you choose, this takes the patients from
ezIdentitySense, the enterprise master patient index, and writes them to a
folder as NDJSON files (one FHIR Patient per line). A data warehouse or
analytics system reads them from there.

It uses ezIdentitySense's FHIR Bulk Data export (`$export`):

1. **Ask** for the patients. ezIdentitySense makes the files in the
   background.
2. **Wait** until they are ready (checked every 10 seconds, for up to two
   hours).
3. **Download** each file into the folder, whole. A file only appears once it
   is complete.
4. **Save the manifest**, last. It lists the run's files, and it is the sign
   that all of them are there.

## What is exported, and when

- **The first run** exports every patient. **Each later run** exports only
  the patients that changed since the last run that **succeeded**, so the
  files are small. Merged patients appear as `active: false`, with a
  `replaced-by` link to the patient they were merged into, so a reader can
  keep its own copy in step.
- **A failed run changes nothing.** The next run starts from the last
  success, so nothing is missed.
- **Runs overlap by 5 minutes** on purpose, so a clock slightly behind
  ezIdentitySense's cannot let a change fall between two runs. A patient can
  therefore appear in two consecutive exports with the same content. A reader
  should **replace the patient it holds by its ID**, not add a second copy.
- **Demographics** (names, birth date and so on) are in the files only if the
  client is allowed `identities.read.demographics` (see below). Without it, a
  patient has only its enterprise ID and its source IDs.

## The files

For a run at 03:00 UTC on 18 November 2026, in the folder you chose:

```
ezidentitysense-patients-20261118T030000Z-part1.ndjson
ezidentitysense-patients-20261118T030000Z-part2.ndjson
ezidentitysense-patients-20261118T030000Z-manifest.json
```

Each file holds up to 50,000 patients. **Read the manifest first**: when it
exists, every part it lists is in the folder. A run with nothing changed has
a manifest and no parts.

Files are not deleted. Clear out old ones on your own schedule.

## Setting up

1. In ezIdentitySense, register a client for ezHealthKonnect with the scopes
   **`identities.export`** and **`identities.read`**, and
   **`identities.read.demographics`** if the files should carry
   demographics. Enter its address, token URL, client ID and secret in the
   form.
2. Choose the **time of day** (24 hour, `HH:MM`) and its **time zone** (an
   IANA name such as `UTC` or `Europe/London`).
3. Choose the **folder**. It is on the server this interface runs on, so a
   network share mounted there works. It is created if it does not exist.
4. To leave demographics out, remove `identities.read.demographics` from
   **Access requested**.

To run every few minutes instead of daily, or also once when the interface
is activated, change the settings of the first step (**Export Schedule**) in
the Pipeline Builder.

## When something goes wrong

- A failed run fails its message, with the reason; see the message's
  processing log. To be told, make an alert rule of the type **Failed
  Messages** for this interface (Alerts, Rules).
- **429 when starting**: ezIdentitySense makes one export at a time for a
  client, and one is still being made (perhaps a run that is still waiting).
  The run fails; the next one covers it.
- **The wait ends without files**: ezIdentitySense stops an export, saying
  why, rather than leave too little free disk. Free some space; the next run
  covers it.
- **403**: the client lacks `identities.export` or `identities.read`.
- ezIdentitySense keeps an export's files only for a limited time (its
  **export link hours** setting). The download starts as soon as they are
  ready, so this matters only if downloads are very slow.

## What this template does not do

It writes to a folder only. Sending the files on to SFTP, S3 or Azure, or
loading them into a FHIR server, is not part of it.
