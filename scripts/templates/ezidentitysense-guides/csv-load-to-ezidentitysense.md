# CSV Load to ezIdentitySense

## What this does

A CSV file of patients, dropped in a folder, on an SFTP server or in an S3
bucket, is loaded into ezIdentitySense, the enterprise master patient index,
through its bulk-load API. Each file goes through the same steps:

1. **Save the column mapping** from the setup form for the source system.
2. **Upload** the file to ezIdentitySense.
3. **Check** it: a dry run reads every row as a load would and registers
   nothing.
4. **Load** it, once the check passes (see the next section).
5. **Report** any refused row, with its line and a reason code.

## Which template to use

| Template | Use it when |
|---|---|
| **CSV Load from a Folder** | The files appear in a folder this server can read: a network share, a mounted drive, or a folder another system writes to. |
| **CSV Load from SFTP** | The source system leaves its files on an SFTP server. This interface fetches them from there. |
| **CSV Load from S3** | The files are in an S3 bucket, or a store with an S3-compatible interface. |

The steps after the file is picked up are identical. In every case a
processed file is moved to the folder you name, so it is not loaded twice.

## If some rows are refused

The setup form asks what to do when the check refuses a row (**If the check
refuses some rows**). Either way the message is marked **failed**, so it
cannot be missed, and the line and reason of every refused row are kept.

| Setting | What happens | Use it when |
|---|---|---|
| **Load nothing: stop and report** (safest) | Nothing from the file is loaded. Fix the file and send the whole file again. | The file is one unit and a partial load would be hard to untangle: a first full load, or a file where rows depend on each other. |
| **Load the good rows and report the rest** | Every row that is fine is loaded. The message still fails, saying the rest were refused. Fix only those rows and send them in a new file. | A routine feed where a few bad rows should not hold up everyone else: a daily extract. |

A row can also be refused during the load itself, though the check makes this
rare. The good rows are still loaded, and the message fails saying so.

A load that ends before it finishes (failed, cancelled or paused) also fails
the message. Whatever it registered stays registered, and a person can resume
it on ezIdentitySense's Bulk load screen.

## What you can see

- **Whether it worked:** the interface's message list. A file that was loaded
  cleanly shows as processed. Any problem shows as failed, with the step it
  stopped at and why, in plain words.
- **Which rows:** open the failed message and its processing log, and look at
  the **Check Rejects** step (or **Load Rejects**). Each entry is a **line
  number** and a **reason code**; the row's contents are not repeated there. Open the file at that
  line and look the code up in the table below.
- **How far a load has got:** ezHealthKonnect shows the message as processing
  while the load runs, without a row count. The row count and the job's state
  are on ezIdentitySense's **Bulk load** screen. Both the check and the load
  appear there as jobs, named after the file.

### Reason codes

| Code | Meaning | What to do |
|---|---|---|
| `invalid-local-id` | The row has no ID, or the ID is longer than 128 characters. | Fill in the column mapped to `localId`. |
| `invalid-version` | The row's version is not a date and time ezIdentitySense can read. | Use a format like `2026-11-18T10:00:00Z`. |
| `invalid-demographics` | The row's values cannot be read as patient details. | Check the mapped fields and values for that row. |
| `insufficient-demographics` | The row does not carry enough to identify a person: it lacks every element of every accepted combination (for example name and birth date). | Add the missing details, or check the column mapping. |
| `invalid-row` | The row is not readable: the wrong number of columns, or broken quoting. | Fix the row's structure; a stray comma or quote is the usual cause. |
| `invalid-encoding` | The row has text that is not in the encoding chosen in the setup form. | Choose the file's real encoding, or re-export it. |
| `unknown-authority` | An identifier names an ID authority ezIdentitySense does not know. | Register the authority in ezIdentitySense, or correct the value. |
| `unknown-source-system` | The source system code is not registered, or is inactive. | Check the code in the setup form. |
| `merged-in-source` | The source has already merged this record into another. | Send the update for the record it was merged into. |
| `idempotency-conflict` | This record version was already received with different content. | Send the row with a new version. |
| `sensitive-identifier-unsupported` | An identifier is from a high-sensitivity authority this installation does not store. | Remove that identifier from the file. |

## Get told when a file fails

ezHealthKonnect can send an alert (email, Slack or a webhook) when messages
on an interface fail. Create the rule once the interface exists:

1. Open **Alerts**, then **Rules**, and create a rule.
2. Choose the type **Failed Messages** and select **this interface**.
3. Set the condition to **Greater than or equal (>=)** with a warning
   threshold of **1**: a single failed file raises the alert. Use a window of
   60 minutes or more, so the failure is still counted when the rule next
   runs.
4. Add a notification channel to the rule.

Alert rules are checked every minute, whether or not anyone has the
monitoring page open. A rule on **Failed Messages** counts only the
interface it is created for. The
other alert types (error rate, validation failures) count every interface
together, and would not notice one failed file among many other messages.

## Setting up

1. In ezIdentitySense, register a client for ezHealthKonnect with the
   **`bulk.load`** scope only (or, for people, the *data loader* role). It
   needs nothing else: it cannot change matching settings or other clients.
   Enter its address, token URL, client ID and secret in the form.
2. Enter the **source system code**: the system these patients belong to, as
   set up in ezIdentitySense.
3. Choose the file's **encoding**.
4. Choose what to do **if the check refuses some rows** (above).
5. Under **Which column holds what**, list the file's own header names, one
   per line, as `HEADER=field`:

   ```
   Patient No=localId
   First=names.0.given.0
   Last=names.0.family.0
   DOB=birthDate
   Sex=sex
   ```

   `localId` is required: it is the record's ID in the source. Include a
   `version` column, or give a fixed one below, so a file sent twice can
   never overwrite newer information. Any field the demographics document
   has is accepted; the load screen in ezIdentitySense lists the usual
   ones.
6. Optionally, under **Fixed values**, give a value every row gets, as
   `field=value`.
7. Fill in where the files are, and where processed ones go.

## How files are taken

- Files are processed **one at a time, in the order received**, so a second
  file waits for the first to finish. ezIdentitySense also runs one bulk job
  at a time: a file that arrives while someone else's job is running fails
  its message, and is sent again later.
- ezIdentitySense runs each job itself, at its own pace, and keeps its place
  in the file if the job is paused or the server restarts. A person can pause,
  resume or cancel a job on its Bulk load screen.
- Waiting for a job is limited to an hour by default. Change **Wait Check**
  or **Wait Load** in the pipeline for a longer wait.

## When something goes wrong

- **The mapping is refused** (the *Save Mapping* step fails): a field name is
  not one the demographics document has, or there is no `localId` column.
- **The upload is refused**: the file is too large (the limit is a setting in
  ezIdentitySense) or is not named `.csv`.
- **A job is refused because another is running**: send the file again when
  the other job has finished.
- **403**: the client lacks the `bulk.load` scope.
