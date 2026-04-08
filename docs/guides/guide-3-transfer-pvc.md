# Guide 3: Transfer-PVC Command — Multi-Cluster Data Migration

You'll write unit tests for the `transfer-pvc` command. This command transfers PVC (Persistent Volume Claim) data from one Kubernetes cluster to another.

By the end, you'll know:
- What PVCs are and why you'd transfer them
- How multi-cluster operations work (kubeconfig and contexts)
- How crane maps source names to destination names
- How command-line flags become struct values
- How to parse and validate user input
- How to test progress tracking and status logic

---

## Part 1: What is a PVC?

**Why you need to know this:** The transfer command moves PVC data. You need to understand what you're moving.

---

### 1.1 Storage in Kubernetes

In Guide 2, you learned about ConfigMaps and Secrets — they store configuration. But what about actual data? Database files, uploaded images, logs?

Kubernetes has a storage system for this. Three concepts:

**PersistentVolume (PV)** — A piece of storage in the cluster. Think of it as a hard drive. It exists independently of any Pod.

**PersistentVolumeClaim (PVC)** — A request for storage. A Pod says "I need 10GB of storage" by creating a PVC. Kubernetes finds a matching PV and binds them together.

**StorageClass** — Defines what kind of storage to provision. "Fast SSD" vs "cheap spinning disk" vs "network storage."

The flow:
1. Admin creates a StorageClass (or uses a default one)
2. User creates a PVC requesting storage
3. Kubernetes provisions a PV and binds it to the PVC
4. Pod mounts the PVC and reads/writes data

---

### 1.2 Why Transfer PVCs?

When migrating an application to a new cluster, you need its data too.

Example: You have a PostgreSQL database in Cluster A. The database stores data in a PVC. You want to move the whole application to Cluster B.

You can export the Kubernetes resource definitions (Deployments, Services, ConfigMaps) with the `export` command from Guide 2. But the actual database files — the bytes on disk — don't come with the YAML. That's what `transfer-pvc` does: it copies the actual data.

---

### 1.3 How Transfer Works (High Level)

The command:
1. Reads the source PVC definition
2. Creates a matching PVC in the destination cluster
3. Sets up encrypted network tunnel between clusters
4. Uses `rsync` to copy all files from source to destination (we'll explain rsync in Part 6)
5. Reports progress in real-time
6. Cleans up temporary resources

You don't need to understand all the networking details. For testing, you'll focus on:
- Parsing user input (names, namespaces, flags)
- Progress tracking (parsing rsync output)
- Validation logic

---

## Part 2: Multi-Cluster Architecture

**Why you need to know this:** The command works with two clusters simultaneously. You need to understand how it connects to both.

---

### 2.1 Kubeconfig and Contexts

Kubernetes runs on a server. To talk to it, your computer needs to know:
- **Where** — The server's address (like `https://my-cluster.example.com:6443`)
- **Who you are** — A certificate or token that proves your identity

This information is stored in a config file, usually at `~/.kube/config`. Tools like `kubectl` (the Kubernetes command-line tool) and crane read this file to connect.

The config file can contain multiple clusters. To avoid typing the address and credentials every time, you create **contexts**. A context is a saved combination of:
- Which cluster to connect to
- Which credentials to use
- Which namespace to use by default

You give each context a name like "production" or "staging", then just say "use context production" instead of specifying everything.

---

### 2.2 How Transfer-PVC Uses Contexts

Example:
```bash
crane transfer-pvc \
  --source-context cluster-a \
  --destination-context cluster-b \
  --pvc-name my-data
```

This tells crane:
- `--source-context cluster-a` — Connect to the cluster saved as "cluster-a" to read data
- `--destination-context cluster-b` — Connect to the cluster saved as "cluster-b" to write data

Crane connects to both clusters simultaneously — reading from source, writing to destination.

---

## Part 3: Mapping Source to Destination

The user might want different names in source and destination. The command supports a mapping format.

---

### 3.1 The Problem

User has a PVC named `database-data` in namespace `production` on Cluster A.

They want to create it as `db-data` in namespace `staging` on Cluster B.

The command needs to accept:
- Source PVC name: `database-data`
- Destination PVC name: `db-data`
- Source namespace: `production`
- Destination namespace: `staging`

---

### 3.2 The Mapping Format

**Why you need to know this:** You'll test the parsing function that handles this format.

The user provides mappings in format `source:destination`:

```bash
crane transfer-pvc \
  --pvc-name database-data:db-data \
  --pvc-namespace production:staging \
  --source-context cluster-a \
  --destination-context cluster-b
```

If source and destination are the same, just one value works:

```bash
--pvc-name database-data
# same as: --pvc-name database-data:database-data
```

---

### 3.3 The mappedNameVar Struct

**Why you need to know this:** This struct stores the parsed mapping. The function you'll test populates it.

```go
type mappedNameVar struct {
    source      string
    destination string
}
```

Two fields:
- **source** — The name in the source cluster
- **destination** — The name in the destination cluster

---

### 3.4 The parseSourceDestinationMapping Function

**Why you need to know this:** This is the function that parses the `source:destination` format from 3.2 and returns values that populate the `mappedNameVar` struct from 3.3.

**Open:** `cmd/transfer-pvc/transfer-pvc.go`, find `parseSourceDestinationMapping`

```go
func parseSourceDestinationMapping(mapping string) (source string, destination string, err error) {
    split := strings.Split(string(mapping), ":")
    switch len(split) {
    case 1:
        if split[0] == "" {
            return "", "", fmt.Errorf("source name cannot be empty")
        }
        return split[0], split[0], nil
    case 2:
        if split[1] == "" || split[0] == "" {
            return "", "", fmt.Errorf("source or destination name cannot be empty")
        }
        return split[0], split[1], nil
    default:
        return "", "", fmt.Errorf("invalid name mapping. must be of format <source>:<destination>")
    }
}
```

The function:
1. Splits the input by `:`
2. If one part: use it for both source and destination
3. If two parts: first is source, second is destination
4. Otherwise: error

---

### 3.5 Existing Tests

**Open:** `cmd/transfer-pvc/transfer-pvc_test.go`

This function already has tests. Read them to understand the test structure used in this package.

Notice:
- Table-driven tests with descriptive names
- Tests for valid inputs AND invalid inputs
- Each test case checks both the returned values and the error

---

### 3.6 Add More Edge Cases

The existing tests are good, but they miss some cases.

**What to do:**

1. Open `cmd/transfer-pvc/transfer-pvc_test.go`
2. Add test cases to the existing `Test_parseSourceDestinationMapping` function:
   - Input with spaces: `"source name:dest name"` — should this work? Test it and see what happens.
   - Input with special characters: `"my-pvc_v1:my-pvc_v2"` — PVC names can have hyphens and underscores.

3. Run: `go test ./cmd/transfer-pvc/... -run Test_parseSourceDestinationMapping -v`

---

## Part 4: Resource Name Validation

Kubernetes has rules about resource names. They can't be too long.

---

### 4.1 The Problem

A user tries to transfer a PVC with a very long name: `my-very-long-application-name-with-lots-of-descriptive-words-database-primary-replica-backup`.

That's 89 characters. Kubernetes resource names must be 63 characters or fewer (DNS label restrictions). The transfer would fail when trying to create resources with this name.

How does crane handle this?

---

### 4.2 The getValidatedResourceName Function

**Why you need to know this:** This function solves the problem from 4.1 — it ensures names fit the 63-character limit.

```go
func getValidatedResourceName(name string) string {
    if len(name) < 63 {
        return name
    } else {
        return fmt.Sprintf("crane-%x", md5.Sum([]byte(name)))
    }
}
```

The function:
1. If the name is under 63 characters, return it unchanged
2. Otherwise, create a hash-based name: `crane-` followed by the MD5 hash

**What is MD5?** It's a function that takes any input and produces a fixed-length output (32 hex characters). The same input always produces the same output. Different inputs produce different outputs. So `crane-` plus 32 hex characters = 38 characters, well under 63.

Example: A 100-character name becomes something like `crane-5d41402abc4b2a76b9719d911017c592`.

---

### 4.3 Write the Test

**What to do:**

1. In `cmd/transfer-pvc/transfer-pvc_test.go`, add a test function for `getValidatedResourceName`
2. Test cases to cover:
   - Short name (under 63 chars): should return unchanged
   - Exactly 62 characters: should return unchanged
   - Exactly 63 characters: look at the condition (`< 63`), so 63 triggers the hash
   - Long name (100 characters): should return hashed version starting with `crane-`
   - Same long name twice: should return the same hash (deterministic)
   - Two different long names: should return different hashes
3. For creating test strings of exact length, use the `strings.Repeat` function
4. For checking the result, verify it starts with `crane-` and is not longer than 63 characters
5. Run: `go test ./cmd/transfer-pvc/... -run Test_getValidatedResourceName -v`

---

## Part 5: Validation Functions

What happens if the user forgets to provide required flags? Or provides invalid combinations? The command should fail early with a clear error, not crash halfway through a transfer.

---

### 5.1 The EndpointFlags Struct

**Why you need to know this:** You'll test this struct's validation method.

**Open:** `cmd/transfer-pvc/transfer-pvc.go`, find `EndpointFlags`.

```go
type EndpointFlags struct {
    Type         endpointType  // "nginx-ingress" or "route"
    Subdomain    string        // Required for nginx-ingress
    IngressClass string        // Optional ingress class
}
```

Three fields:
- **Type** — Which endpoint type to use
- **Subdomain** — The subdomain for the ingress (e.g., "my-app.example.com")
- **IngressClass** — Optional: which ingress class to use (e.g., "nginx", "traefik", "haproxy")

---

### 5.2 The endpointType Custom Type

**Why you need to know this:** The `Type` field uses this custom type. You need to understand what values it can have.

```go
type endpointType string

const (
    endpointNginx endpointType = "nginx-ingress"
    endpointRoute endpointType = "route"
)
```

This is a custom string type with two allowed values:
- `endpointNginx` — the string "nginx-ingress" (uses Kubernetes Ingress)
- `endpointRoute` — the string "route" (uses OpenShift Routes)

---

### 5.3 The EndpointFlags.Validate Method

**Why you need to know this:** This is the method you'll test.

```go
func (e EndpointFlags) Validate() error {
    if e.Type == "" {
        e.Type = endpointNginx
    }
    switch e.Type {
    case endpointNginx:
        if e.Subdomain == "" {
            return fmt.Errorf("subdomain cannot be empty when using nginx ingress")
        }
    }
    return nil
}
```

The logic:
1. Default to nginx-ingress if type is empty
2. If nginx-ingress, subdomain is required
3. Routes don't need a subdomain

---

### 5.4 Write the Test for EndpointFlags.Validate

**What to do:**

1. Add a test function for `EndpointFlags.Validate` to the test file
2. Test cases to cover:
   - nginx-ingress with subdomain: should return nil (valid)
   - nginx-ingress without subdomain: should return error
   - route without subdomain: should return nil (valid)
   - route with subdomain: should return nil (subdomain is ignored, not an error)
   - empty type with subdomain: should return nil (defaults to nginx-ingress, which needs subdomain — this satisfies it)
   - empty type without subdomain: should return error (defaults to nginx-ingress, which needs subdomain)
3. Run: `go test ./cmd/transfer-pvc/... -run Test_EndpointFlags_Validate -v`

---

### 5.5 The PvcFlags Struct

**Why you need to know this:** You'll test this struct's validation method.

**Open:** `cmd/transfer-pvc/transfer-pvc.go`, find `PvcFlags`.

```go
type PvcFlags struct {
    Name             mappedNameVar  // source:destination PVC names (from Part 3)
    Namespace        mappedNameVar  // source:destination namespaces
    StorageClassName string         // Optional: storage class for destination
    StorageRequests  quantityVar    // Optional: storage size
}
```

Four fields:
- **Name** — The PVC name mapping (uses `mappedNameVar` you learned in Part 3)
- **Namespace** — The namespace mapping (same type)
- **StorageClassName** — Optional: override the storage class in destination
- **StorageRequests** — Optional: override the storage size

---

### 5.6 The quantityVar Custom Type

**Why you need to know this:** The `StorageRequests` field uses this type. You need to understand what it is.

```go
type quantityVar struct {
    quantity *resource.Quantity
}
```

This wraps a Kubernetes `resource.Quantity` — a type that represents amounts like "10Gi" (10 gibibytes), "500Mi" (500 mebibytes), etc. Kubernetes uses this for storage sizes, memory limits, and CPU requests.

The pointer means it can be nil (not set by user).

---

### 5.7 The PvcFlags.Validate Method

**Why you need to know this:** This is the method you'll test.

```go
func (p *PvcFlags) Validate() error {
    if p.Name.source == "" {
        return fmt.Errorf("source pvc name cannot be empty")
    }
    if p.Name.destination == "" {
        return fmt.Errorf("destnation pvc name cannot be empty")
    }
    if p.Namespace.source == "" {
        return fmt.Errorf("source pvc namespace cannot be empty")
    }
    if p.Namespace.destination == "" {
        return fmt.Errorf("destination pvc namespace cannot be empty")
    }
    return nil
}
```

All four fields must be non-empty: source name, destination name, source namespace, destination namespace.

---

### 5.8 Write the Test for PvcFlags.Validate

**What to do:**

1. Add a test function for `PvcFlags.Validate`
2. Test cases to cover:
   - All fields set: should return nil
   - Missing source name: should return error containing "source pvc name"
   - Missing destination name: should return error containing "destnation" (note the typo in the code — test what the code actually does)
   - Missing source namespace: should return error
   - Missing destination namespace: should return error
   - Only names set, no namespaces: should return error (check which error comes first)
3. Run: `go test ./cmd/transfer-pvc/... -run Test_PvcFlags_Validate -v`

---

### 5.9 How Flags Become Struct Values

**Why you need to know this:** Now that you've tested validation, here's context on how user input gets into these structs.

When you run:
```bash
crane transfer-pvc --pvc-name database:db --endpoint nginx-ingress
```

The CLI framework (Cobra) does the following:

1. Parses the command line and extracts flag values
2. For `--pvc-name database:db`, calls `parseSourceDestinationMapping` (which you tested in Part 3)
3. Stores the result in a struct field
4. Before running the transfer, calls `Validate()` on each struct — this is what you just tested

The tests you wrote create structs directly and call `Validate()`. You're testing step 4, not the parsing steps.

---

## Part 6: Progress Tracking

When transferring gigabytes of data, the user needs to know: How much is done? How fast? Are there errors? The transfer shows real-time progress by parsing output from the tool that copies the files.

---

### 6.1 What is rsync?

**Why you need to know this:** The progress parser reads rsync output. You need to understand what it's parsing.

**Rsync** is a command-line tool that copies files between two locations. It's commonly used for backups and transfers because:
- It only copies files that changed (efficient)
- It can resume interrupted transfers
- It works over the network

Crane uses rsync to copy data from the source PVC to the destination PVC. While rsync runs, it prints progress information that crane parses.

---

### 6.2 Rsync Output Format

**Why you need to know this:** This is what the parser reads. You need to understand the format to test parsing.

When running, rsync outputs progress like:

```
16.78M   3%  105.06MB/s    0:00:00 (xfr#1, to-chk=19/21)
33.55M   6%   86.40MB/s    0:00:00 (xfr#2, to-chk=18/21)
```

This line means:
- `33.55M` — 33.55 megabytes transferred so far
- `6%` — 6% complete
- `86.40MB/s` — Current transfer speed
- `(xfr#2, to-chk=18/21)` — Transferred 2 files, 18 of 21 left to check. Rsync checks each file as it goes — if it's already up to date, no transfer needed. So 3 files checked, 2 transferred, 1 was already up to date.

When rsync finishes, it outputs stats:
```
Number of files: 136 (reg: 130, dir: 6)
Number of regular files transferred: 130
Total transferred file size: 8.67G bytes
```

The progress parser extracts these values from the text output.

---

### 6.3 The dataSize Struct

**Why you need to know this:** This struct represents data amounts like "33.55 MB". The parsing functions use it.

```go
type dataSize struct {
    val  float64  // The number (33.55)
    unit string   // The unit ("M", "G", "K", "bytes")
}
```

Two fields:
- **val** — The numeric value as a decimal number
- **unit** — The unit string ("M" for megabytes, "G" for gigabytes, etc.)

---

### 6.4 The newDataSize Function

**Why you need to know this:** This function parses strings like "33.55M" into the `dataSize` struct from 6.3. You'll test this function.

```go
func newDataSize(str string) *dataSize {
    r := regexp.MustCompile(`([\d\.]+)([\w\/]*)`)
    matched := r.FindStringSubmatch(str)
    if len(matched) < 2 {
        return nil
    }
    size, err := strconv.ParseFloat(matched[1], 64)
    if err != nil {
        return nil
    }
    unit := matched[2]
    if unit == "" {
        unit = "bytes"
    }
    return &dataSize{
        val:  size,
        unit: unit,
    }
}
```

The function:
1. Uses a regex pattern to extract the number and unit parts
2. Parses the number as float64
3. Defaults unit to "bytes" if none provided
4. Returns nil if parsing fails

---

### 6.5 Write Test for newDataSize

**What to do:**

1. In `cmd/transfer-pvc/progress_test.go`, add a test function for `newDataSize`
2. Test cases to cover:
   - "33.55M": should return val=33.55, unit="M"
   - "100G": should return val=100, unit="G"
   - "1024K": should return val=1024, unit="K"
   - "86.40MB/s": should return val=86.40, unit="MB/s" (rate includes /s)
   - "1024": should return val=1024, unit="bytes" (no unit defaults to bytes)
   - "": should return nil
   - "abc": should return nil (no number to parse)
3. Run: `go test ./cmd/transfer-pvc/... -run Test_newDataSize -v`

---

### 6.6 The addDataSize Function

**Why you need to know this:** When a transfer fails and retries, progress from multiple attempts needs to be combined. This function adds two `dataSize` values together, handling unit conversion.

```go
func addDataSize(a, b *dataSize) *dataSize {
    if b == nil {
        return nil
    }
    newDs := &dataSize{}
    units := map[string]int{"bytes": 0, "K": 3, "M": 6, "G": 9, "T": 12}
    if b.unit == a.unit {
        newDs.val = b.val + a.val
        newDs.unit = b.unit
    } else {
        if nu, exists := units[b.unit]; exists {
            if du, exists := units[a.unit]; exists {
                if nu > du {
                    newDs.val = b.val + (a.val / math.Pow(10, float64(nu-du)))
                    newDs.unit = b.unit
                } else {
                    newDs.val = (b.val / math.Pow(10, float64(du-nu))) + a.val
                    newDs.unit = a.unit
                }
            }
        }
    }
    return newDs
}
```

The function adds two data sizes, converting units if needed. The units map shows each unit is 1000x the previous (K=1000 bytes, M=1000K, etc.).

---

### 6.7 Write Test for addDataSize

**What to do:**

1. Add a test function for `addDataSize`
2. Test cases to cover:
   - Same units: 10M + 5M should equal 15M
   - Different units: think about what 1G + 500M should produce
   - b is nil: should return nil
   - a is nil: test what actually happens
3. You'll need a helper function to compare two dataSize pointers (check if both nil, or both have same val and unit)
4. Run: `go test ./cmd/transfer-pvc/... -run Test_addDataSize -v`

---

## Part 7: Transfer Status

The user needs to know: Is the transfer still running? Did it succeed? Did it fail? The progress tracker reports status based on what's happened so far.

---

### 7.1 The FailedFile Struct

**Why you need to know this:** The `Progress` struct uses this to track files that failed to transfer.

```go
type FailedFile struct {
    Name string  // Path of the file that failed
    Err  string  // Error message
}
```

Two fields:
- **Name** — The file path that couldn't be transferred
- **Err** — Why it failed (e.g., "Permission denied")

---

### 7.2 The types.NamespacedName Type

**Why you need to know this:** The `Progress` struct uses this to identify which PVC is being transferred.

`types.NamespacedName` comes from the Kubernetes client library. It's a simple struct:

```go
type NamespacedName struct {
    Namespace string
    Name      string
}
```

It uniquely identifies a resource by its namespace and name.

---

### 7.3 The Progress Struct

**Why you need to know this:** You'll test its `Status` method. You need to understand its fields.

```go
type Progress struct {
    PVC                types.NamespacedName  // Which PVC (from 7.2)
    TransferPercentage *int64                // 0-100, nil if not started
    TransferRate       *dataSize             // Speed (from 6.3)
    TransferredData    *dataSize             // Amount moved (from 6.3)
    TotalFiles         *int64                // Total file count
    TransferredFiles   int64                 // Files successfully moved
    ExitCode           *int32                // rsync exit code, nil if still running
    FailedFiles        []FailedFile          // Files that failed (from 7.1)
    Errors             []string              // Error messages
    retries            *int                  // Number of retry attempts (private)
    startedAt          time.Time             // When transfer started
}
```

Key fields for determining status:
- **TransferPercentage** — How complete the transfer is
- **ExitCode** — rsync's exit code (nil = still running, 0 = success, other = failure)
- **TransferredFiles** — Number of files successfully transferred
- **TransferredData** — Amount of data moved
- **TotalFiles** — Total number of files to transfer

---

### 7.4 Why Are Some Fields Pointers?

**Why you need to know this:** The Progress struct has both pointer fields (`*int64`) and non-pointer fields (`int64`). You need to understand the difference to write correct tests.

Look at `TransferPercentage *int64` vs `TransferredFiles int64`.

`*int64` is a pointer. It can be:
- `nil` — We don't know the value yet
- A pointer to an actual number

`int64` (no pointer) always has a value. In Go, uninitialized integers are 0.

Using pointers lets you distinguish "we know it's 0" from "we don't know yet."

In the Progress struct:
- `TransferPercentage *int64` — We don't know until rsync reports it. nil means "not yet known."
- `TransferredFiles int64` — Starts at 0, which is meaningful (0 files transferred so far).

When writing tests, you'll need to create pointer values. You can't write `&100` (address of a literal). Instead:
```go
int100 := int64(100)
progress.TransferPercentage = &int100
```

---

### 7.5 The status Type and Constants

**Why you need to know this:** The `Status` method returns one of these values.

```go
type status string

const (
    succeeded          status = "Succeeded"
    failed             status = "Failed"
    partiallyFailed    status = "Partially failed"
    preparing          status = "Preparing"
    transferInProgress status = "Transfer in-progress"
    finishingUp        status = "Finishing up"
)
```

Six possible states:
- **preparing** — Transfer hasn't really started yet
- **transferInProgress** — Actively copying files
- **finishingUp** — Reached 100% but rsync hasn't exited yet
- **succeeded** — Completed successfully (exit code 0)
- **failed** — Nothing transferred, non-zero exit code
- **partiallyFailed** — Some files transferred, but errors occurred

---

### 7.6 The Status Method

**Why you need to know this:** This is the method you'll test. It determines the current status based on the Progress fields.

```go
func (p *Progress) Status() status {
    if p.ExitCode != nil {
        if *p.ExitCode == 0 {
            int100 := int64(100)
            p.TransferPercentage = &int100
            return succeeded
        }
        if p.TransferredFiles == 0 &&
            p.TransferredData.val == 0 &&
            p.TotalFiles == nil {
            return failed
        }
        return partiallyFailed
    } else {
        if p.TransferPercentage == nil {
            return preparing
        }
        if *p.TransferPercentage >= 100 {
            return finishingUp
        }
    }
    return transferInProgress
}
```

The logic:
1. If ExitCode is set (transfer finished):
   - Exit code 0 = succeeded
   - Exit code non-zero with nothing transferred = failed
   - Exit code non-zero with some data transferred = partiallyFailed
2. If ExitCode is NOT set (still running):
   - No percentage yet = preparing
   - Percentage >= 100 = finishingUp
   - Otherwise = transferInProgress

---

### 7.7 Write Test for Status

**What to do:**

1. Add a test function for `Progress.Status`
2. Think through each status and what Progress fields would produce it:
   - succeeded: exit code is 0
   - failed: exit code is non-zero, but nothing was transferred (TransferredFiles=0, TransferredData.val=0, TotalFiles=nil)
   - partiallyFailed: exit code is non-zero, but some files were transferred
   - preparing: no exit code yet, no percentage yet
   - finishingUp: no exit code yet, but percentage is 100 or more
   - transferInProgress: no exit code, percentage is set but under 100
3. For each case, create a Progress struct with the right fields set
4. You'll need helper functions to create pointers to int32 and int64 values
5. Run: `go test ./cmd/transfer-pvc/... -run Test_Progress_Status -v`

---

## Part 8: Parsing Rsync Logs

The existing test file has tests for `parseRsyncLogs`. Let's understand and extend them.

---

### 8.1 Review Existing Tests

**Open:** `cmd/transfer-pvc/progress_test.go`

The `Test_parseRsyncLogs` function tests parsing of rsync output. Study how it:
- Creates raw log strings (stdout, stderr)
- Calls `parseRsyncLogs`
- Checks the returned Progress struct fields
- Verifies the status
- Checks for unprocessed text

---

### 8.2 Add Test for Error Parsing

The parser extracts error messages from rsync output using regex patterns. One pattern catches lines like `@ERROR: access denied`.

**What to do:**

1. Add a test case to `Test_parseRsyncLogs` that includes `@ERROR:` lines in stdout
2. Verify the `Errors` slice in the returned Progress contains the error messages
3. Add an assertion to the test loop that checks the Errors field
4. Run: `go test ./cmd/transfer-pvc/... -run Test_parseRsyncLogs -v`

---

### 8.3 Add Test for Retry Detection

The parser detects when rsync retries using a regex that matches lines like `Syncronization failed. Retrying in 5 seconds. Retry 2/3`.

**What to do:**

1. Add a test case with a retry message in stdout
2. The `retries` field is private, so you can't directly check it. For now, verify the test doesn't panic and returns the expected status.
3. Run: `go test ./cmd/transfer-pvc/... -run Test_parseRsyncLogs -v`

---

## Part 9: The Completed Method

How does code know when to stop waiting for a transfer? It needs to know if the status represents a "done" state.

---

### 9.1 The Completed Method

**Why you need to know this:** This helper method answers "is the transfer done?" regardless of whether it succeeded or failed.

```go
func (s status) Completed() bool {
    return s == succeeded || s == failed || s == partiallyFailed
}
```

Returns true for the three "done" states (succeeded, failed, partiallyFailed), false for the three "still going" states (preparing, transferInProgress, finishingUp).

---

### 9.2 Write the Test

**What to do:**

1. Add a test function for `status.Completed`
2. Test all six status values:
   - succeeded: should return true
   - failed: should return true
   - partiallyFailed: should return true
   - preparing: should return false
   - transferInProgress: should return false
   - finishingUp: should return false
3. Run: `go test ./cmd/transfer-pvc/... -run Test_status_Completed -v`

---

## Summary

You learned:
- What PVCs are (persistent storage in Kubernetes)
- How multi-cluster operations work (kubeconfig and contexts)
- Source-to-destination name mapping format
- How command-line flags become struct values (Cobra binding)
- Kubernetes 63-character name limit and how crane handles it
- Endpoint types (nginx-ingress vs route) and their validation
- How rsync output is parsed into structured progress data
- The difference between pointer and non-pointer fields
- Transfer status states and their transitions

You wrote tests for:
- `getValidatedResourceName`
- `EndpointFlags.Validate`
- `PvcFlags.Validate`
- `newDataSize`
- `addDataSize`
- `Progress.Status`
- `status.Completed`
- Extended `parseRsyncLogs` tests

---

## Next Steps

1. Run all transfer-pvc tests: `go test ./cmd/transfer-pvc/... -v`
2. Make sure they pass
3. Consider edge cases you might have missed
4. Look at the functions that interact with Kubernetes clients — those require mocking (advanced topic from Guide 2 Part 7)
