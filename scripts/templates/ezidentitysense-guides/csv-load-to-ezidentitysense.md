# CSV Load to ezIdentitySense

## What this does

A CSV file of patients, dropped in a folder, on an SFTP server or in an S3
bucket, is loaded into ezIdentitySense, the enterprise master patient index,
through its bulk-load API. Each file goes through the same steps:

1. **Save the column mapping** from the setup form for the source system.
2. **Upload** the file to ezIdentitySense.
3. **Check** it: a dry run reads every row as a load would and registers
   nothing.
4. **Load** it, only if the check refused no rows. ezIdentitySense registers
   the rows in file order.
5. **Report** any refused row, with its line and the reason.

If the check refuses any row, **nothing is loaded**, and the message shows as
failed with the reason for each row in the *Check Rejects* step. Fix the file
and send it again. A load that ends before it finishes (failed, cancelled or
paused) also fails the message, and says so.

## Which template to use

| Template | Use it when |
|---|---|
| **CSV Load from a Folder** | The files appear in a folder this server can read: a network share, a mounted drive, or a folder another system writes to. |
| **CSV Load from SFTP** | The source system leaves its files on an SFTP server. This interface fetches them from there. |
| **CSV Load from S3** | The files are in an S3 bucket, or a store with an S3-compatible interface. |

The steps after the file is picked up are identical. In every case a
processed file is moved to the folder you name, so it is not loaded twice.

## Setting up

1. In ezIdentitySense, register a client for ezHealthKonnect with the
   **`bulk.load`** scope only (or, for people, the *data loader* role). It
   needs nothing else: it cannot change matching settings or other clients.
   Enter its address, token URL, client ID and secret in the form.
2. Enter the **source system code**: the system these patients belong to, as
   set up in ezIdentitySense.
3. Choose the file's **encoding**.
4. Under **Which column holds what**, list the file's own header names, one
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
5. Optionally, under **Fixed values**, give a value every row gets, as
   `field=value`.
6. Fill in where the files are, and where processed ones go.

## How files are taken

- Files are processed **one at a time, in the order received**, so a second
  file waits for the first to finish. ezIdentitySense also runs one bulk job
  at a time.
- ezIdentitySense runs each job itself, at its own pace, and keeps its place
  in the file if the job is paused or the server restarts. A person can pause,
  resume or cancel a job on its Bulk load screen. If a job ends before it
  finishes, the message fails and says which state it ended in; what the job
  registered stays registered.
- Waiting for a job is limited to an hour by default. Change **Wait Check**
  or **Wait Load** in the pipeline for a longer wait.

## When something goes wrong

- **"The check refused some rows"**: open the *Check Rejects* step. Each
  entry is a line number and a reason code. Nothing was loaded.
- **The mapping is refused** (the *Save Mapping* step fails): a field name is
  not one the demographics document has, or there is no `localId` column.
- **The upload is refused**: the file is too large (the limit is a setting in
  ezIdentitySense) or is not named `.csv`.
- **A job is refused because another is running**: ezIdentitySense runs one
  bulk job at a time. The message fails; send the file again when the other
  job has finished.
- **403**: the client lacks the `bulk.load` scope.
