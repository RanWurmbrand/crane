# Guide 3: Transfer-PVC Command — Multi-Cluster Operations

You'll write unit tests for the `transfer-pvc` command. This command copies data between two Kubernetes clusters using rsync.

By the end, you'll know:
- What PersistentVolumeClaims (PVCs) are
- How multi-cluster operations work
- How to parse and test rsync log output
- Advanced testing patterns for complex state

---

## Part 1: What is a PersistentVolumeClaim?

**Why you need to know this:** The entire command is about transferring PVC data. You can't test it without understanding what PVCs are.

---

### 1.1 The Problem PVCs Solve

Containers are temporary. When a container restarts, any files it created are gone. But applications need to store data that survives restarts — databases, uploads, logs.

Kubernetes solves this with "volumes" — storage that exists outside the container and survives restarts.

---

### 1.2 How Storage Works in Kubernetes

Three concepts work together:

1. **PersistentVolume (PV)** — A piece of actual storage (disk space on a server, cloud storage, network drive)
2. **PersistentVolumeClaim (PVC)** — A request for storage. Your application says "I need 10GB of fast storage"
3. **StorageClass** — Defines what kind of storage is available (fast SSD, slow HDD, network storage)

The flow:
1. You create a PVC: "I need 10GB"
2. Kubernetes finds or creates a PV that matches
3. Your Pod mounts the PVC and reads/writes files to it

---

### 1.3 Why Transfer PVCs?

When migrating applications between clusters, you need to:
1. Export the Kubernetes resource definitions (what you did with the export command)
2. Transfer the actual data stored in PVCs

The transfer-pvc command handles step 2. It:
1. Connects to the source cluster, finds the PVC
2. Creates an equivalent PVC on the destination cluster
3. Uses rsync to copy files from source to destination

---

### 1.4 The PVC Data Structure

In Go, a PVC is represented by `corev1.PersistentVolumeClaim`:

```go
type PersistentVolumeClaim struct {
    metav1.TypeMeta
    metav1.ObjectMeta     // name, namespace, labels
    Spec   PersistentVolumeClaimSpec
    Status PersistentVolumeClaimStatus
}

type PersistentVolumeClaimSpec struct {
    AccessModes      []PersistentVolumeAccessMode  // ReadWriteOnce, ReadOnlyMany, etc.
    Resources        ResourceRequirements          // how much storage requested
    StorageClassName *string                       // which StorageClass to use
    VolumeName       string                        // specific PV to bind to
    // ... more fields
}
```

Key fields:
- **Name/Namespace** — Identifies the PVC
- **AccessModes** — Can multiple pods read? Can they write? `ReadWriteOnce` = one pod can read and write
- **Resources.Requests** — How much storage (e.g., "10Gi")
- **StorageClassName** — What type of storage

---

## Part 2: Multi-Cluster Architecture

**Why you need to know this:** The command works with two clusters simultaneously.

---

### 2.1 Kubeconfig and Contexts

When you interact with Kubernetes, you use a config file (`~/.kube/config` by default). This file can contain:
- Multiple clusters (URLs to connect to)
- Multiple users (credentials)
- Multiple contexts (cluster + user + default namespace)

A "context" is a shortcut: instead of specifying cluster, user, and namespace every time, you say "use context X."

---

### 2.2 How Transfer-PVC Uses Contexts

The command takes two flags:
- `--source-context` — The context for the cluster to copy FROM
- `--destination-context` — The context for the cluster to copy TO

It creates two separate clients — one for each cluster — and orchestrates the transfer.

---

### 2.3 The TransferPVCCommand Struct

**Open:** `cmd/transfer-pvc/transfer-pvc.go`

```go
type TransferPVCCommand struct {
    configFlags *genericclioptions.ConfigFlags
    genericclioptions.IOStreams
    logger logrus.FieldLogger

    sourceContext      *clientcmdapi.Context
    destinationContext *clientcmdapi.Context

    Flags
}
```

Fields explained:
- **configFlags** — Helper to read kubeconfig
- **IOStreams** — Standard input/output streams (for logging)
- **logger** — Logging interface
- **sourceContext** — The parsed source context
- **destinationContext** — The parsed destination context
- **Flags** — Command-line options (embedded struct)

---

### 2.4 The Flags Struct

```go
type Flags struct {
    PVC                PvcFlags
    Endpoint           EndpointFlags
    SourceContext      string
    DestinationContext string
    SourceImage        string
    DestinationImage   string
    Verify             bool
    RsyncFlags         []string
    ProgressOutput     string
}
```

Nested structs:

```go
type PvcFlags struct {
    Name             mappedNameVar   // source:destination mapping
    Namespace        mappedNameVar   // source:destination mapping
    StorageClassName string
    StorageRequests  quantityVar     // storage amount
}

type EndpointFlags struct {
    Type         endpointType    // "nginx-ingress" or "route"
    Subdomain    string
    IngressClass string
}
```

---

## Part 3: Study the Existing Tests

Before writing new tests, understand the existing ones.

---

### 3.1 Test for parseSourceDestinationMapping

**Open:** `cmd/transfer-pvc/transfer-pvc_test.go`

This tests a pure function that parses strings like "source:destination" into two separate values.

```go
func Test_parseSourceDestinationMapping(t *testing.T) {
    tests := []struct {
        name            string
        mapping         string
        wantSource      string
        wantDestination string
        wantErr         bool
    }{
        // test cases...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            gotSource, gotDestination, err := parseSourceDestinationMapping(tt.mapping)
            // assertions...
        })
    }
}
```

Notice the pattern:
- Table-driven test with named cases
- `t.Run(tt.name, ...)` — Creates a subtest for each case
- Tests both successful parsing AND error cases

---

### 3.2 What t.Run Does

**Why you need to know this:** The existing tests use subtests. You'll use them too.

`t.Run(name, func(t *testing.T) {...})` creates a subtest with its own name. Benefits:
- Each case shows separately in output
- A failure in one case doesn't stop other cases
- You can run a specific case: `go test -run TestName/casename`

---

### 3.3 Testing Functions That Return Errors

Look at how the test handles errors:

```go
if (err != nil) != tt.wantErr {
    t.Errorf("parseSourceDestinationMapping() error = %v, wantErr %v", err, tt.wantErr)
    return
}
```

This pattern:
- `(err != nil)` — True if there WAS an error
- `tt.wantErr` — True if we EXPECTED an error
- If these don't match, the test fails

The `return` after `t.Errorf` stops checking other assertions if the error expectation was wrong.

---

## Part 4: Test parseRsyncLogs

This function parses rsync output to extract progress information. It's complex but testable.

---

### 4.1 Understand What Rsync Outputs

Rsync is a file synchronization tool. When running, it outputs progress like:

```
16.78M   3%  105.06MB/s    0:00:00 (xfr#1, to-chk=19/21)
33.55M   6%   86.40MB/s    0:00:00 (xfr#2, to-chk=18/21)
```

This line means:
- `33.55M` — 33.55 megabytes transferred so far
- `6%` — 6% complete
- `86.40MB/s` — Current transfer speed
- `(xfr#2, to-chk=18/21)` — Transferred 2 files, 18 of 21 remaining

When rsync finishes, it outputs stats:
```
Number of files: 136 (reg: 130, dir: 6)
Number of regular files transferred: 130
Total transferred file size: 8.67G bytes
```

---

### 4.2 The Progress Struct

**Open:** `cmd/transfer-pvc/progress.go`

```go
type Progress struct {
    PVC                types.NamespacedName `json:"pvc"`
    TransferPercentage *int64               `json:"transferPercentage"`
    TransferRate       *dataSize            `json:"transferRate"`
    TransferredData    *dataSize            `json:"transferredData"`
    TotalFiles         *int64               `json:"totalFiles"`
    TransferredFiles   int64                `json:"transferredFiles"`
    ExitCode           *int32               `json:"exitCode"`
    FailedFiles        []FailedFile         `json:"failedFiles"`
    Errors             []string             `json:"miscErrors"`
    retries            *int
    startedAt          time.Time
}
```

Fields explained:
- **PVC** — Which PVC is being transferred
- **TransferPercentage** — How far along (0-100). Pointer because it might be unknown (nil)
- **TransferRate** — Speed of transfer (e.g., "86.40 MB/s")
- **TransferredData** — Total data transferred so far
- **TotalFiles** — Number of files to transfer
- **TransferredFiles** — How many files transferred so far
- **ExitCode** — rsync exit code when done. 0 = success
- **FailedFiles** — Files that failed to transfer
- **Errors** — Other errors that occurred

---

### 4.3 Why Are Some Fields Pointers?

Look at `TransferPercentage *int64` vs `TransferredFiles int64`.

`*int64` is a pointer. It can be:
- `nil` — We don't know the value yet
- A pointer to an actual number

`int64` (no pointer) always has a value. In Go, uninitialized integers are 0.

Using pointers lets you distinguish "we know it's 0" from "we don't know yet."

In the Progress struct:
- `TransferPercentage *int64` — We don't know until rsync reports it
- `TransferredFiles int64` — Starts at 0, which is meaningful (0 files transferred)

---

### 4.4 The dataSize Struct

```go
type dataSize struct {
    val  float64
    unit string
}
```

Represents amounts like "33.55M" or "8.67G":
- `val` — The numeric part (33.55)
- `unit` — The unit ("M" for megabytes, "G" for gigabytes)

---

### 4.5 Study the Existing Test

**Open:** `cmd/transfer-pvc/progress_test.go`

```go
func Test_parseRsyncLogs(t *testing.T) {
    int130 := int64(130)
    // ... more variables
    tests := []struct {
        name            string
        stdout          string
        stderr          string
        want            Progress
        wantStatus      status
        wantUnProcessed string
    }{
        // test cases...
    }
    // loop and assertions...
}
```

Notice:
- Variables like `int130 := int64(130)` are created outside the test cases. This is because you can't take the address of a literal (`&130` doesn't work), but you can take the address of a variable (`&int130`).
- The test cases include raw rsync output as multiline strings
- Helper functions `intEqual` and `dataEqual` handle nil comparisons

---

### 4.6 Write Your Own Test Case

**What to do:**

1. Open `cmd/transfer-pvc/progress_test.go`
2. Add a new test case to the `tests` slice in `Test_parseRsyncLogs`
3. Test this scenario: rsync reports an error for a file

   Use this raw output:
   ```
   rsync: [sender] send_files failed to open "/data/secret.txt": Permission denied (13)
   ```

4. Your expected `Progress` should have:
   - One entry in `FailedFiles` with Name="/data/secret.txt" and Err="Permission denied (13)"

5. Run: `go test ./cmd/transfer-pvc/... -run Test_parseRsyncLogs -v`

---

## Part 5: Test the Status Method

The `Status()` method returns the current state based on Progress fields.

---

### 5.1 Understand the Status Logic

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

func (p *Progress) Status() status {
    if p.ExitCode != nil {
        if *p.ExitCode == 0 {
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
- If ExitCode is set:
  - 0 = succeeded
  - Non-zero with no progress = failed
  - Non-zero with some progress = partiallyFailed
- If ExitCode is not set:
  - No percentage yet = preparing
  - 100% = finishingUp
  - Otherwise = transferInProgress

---

### 5.2 Write Status Tests

**What to do:**

1. Create a new test function `TestProgress_Status(t *testing.T)` in `progress_test.go`
2. Test cases to include:
   - ExitCode 0: expect succeeded
   - ExitCode non-zero, no files transferred: expect failed
   - ExitCode non-zero, some files transferred: expect partiallyFailed
   - No ExitCode, no percentage: expect preparing
   - No ExitCode, percentage 100: expect finishingUp
   - No ExitCode, percentage 50: expect transferInProgress

3. For each case, create a `Progress` struct with appropriate fields set
4. Call `p.Status()` and compare to expected status

5. Run: `go test ./cmd/transfer-pvc/... -run TestProgress_Status -v`

---

## Part 6: Test addDataSize

This function adds two dataSize values, handling unit conversions.

---

### 6.1 Understand the Function

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
        // unit conversion logic...
    }
    return newDs
}
```

The `units` map stores the power of 10 for each unit:
- "bytes" = 10^0 = 1
- "K" = 10^3 = 1,000
- "M" = 10^6 = 1,000,000
- etc.

When units differ, it converts to the larger unit.

---

### 6.2 Write addDataSize Tests

**What to do:**

1. Create `TestAddDataSize(t *testing.T)` in `progress_test.go`
2. Test cases:
   - Same units: 10M + 5M = 15M
   - Different units: 500K + 1M = 1.5M (approximately)
   - Nil input: expect nil output
   - Zero values

3. Run: `go test ./cmd/transfer-pvc/... -run TestAddDataSize -v`

---

## Part 7: Test newDataSize

This function parses strings like "33.55M" into dataSize structs.

---

### 7.1 Understand the Function

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

The regex `([\d\.]+)([\w\/]*)`:
- `([\d\.]+)` — Captures digits and dots (the number)
- `([\w\/]*)` — Captures word characters and slashes (the unit, like "MB/s")

---

### 7.2 Write newDataSize Tests

**What to do:**

1. Create `TestNewDataSize(t *testing.T)` in `progress_test.go`
2. Test cases:
   - "33.55M" → val: 33.55, unit: "M"
   - "8.67G" → val: 8.67, unit: "G"
   - "86.40MB/s" → val: 86.40, unit: "MB/s"
   - "100" → val: 100, unit: "bytes" (default)
   - "invalid" → nil
   - "" → nil

3. Run: `go test ./cmd/transfer-pvc/... -run TestNewDataSize -v`

---

## Part 8: Test getValidatedResourceName

A simpler function that validates resource names.

---

### 8.1 Understand the Function

```go
func getValidatedResourceName(name string) string {
    if len(name) < 63 {
        return name
    } else {
        return fmt.Sprintf("crane-%x", md5.Sum([]byte(name)))
    }
}
```

Kubernetes resource names must be 63 characters or less. If the name is too long, it creates a hash-based name.

---

### 8.2 Write the Test

**What to do:**

1. Add `TestGetValidatedResourceName(t *testing.T)` to `transfer-pvc_test.go`
2. Test cases:
   - Short name (under 63 chars): returns unchanged
   - Exactly 62 chars: returns unchanged
   - Exactly 63 chars: returns hash-based name
   - Long name (over 63 chars): returns hash-based name

3. Verify the hash-based name:
   - Starts with "crane-"
   - Is deterministic (same input = same output)

4. Run: `go test ./cmd/transfer-pvc/... -run TestGetValidatedResourceName -v`

---

## Part 9: Testing Functions That Need Clients (Advanced)

Some functions require a Kubernetes client. These are harder to test.

---

### 9.1 Functions That Need Mocking

These functions call `client.Get`, `client.List`, etc.:
- `getNodeNameForPVC` — Lists pods to find which node has the PVC
- `getIDsForNamespace` — Gets namespace to read security annotations
- `getRsyncClientPodSecurityContext` — Uses getIDsForNamespace
- `getRsyncServerPodSecurityContext` — Uses getIDsForNamespace

---

### 9.2 Using controller-runtime Fake Client

The controller-runtime library provides a fake client:

```go
import (
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "k8s.io/client-go/kubernetes/scheme"
)

// Create fake client with pre-populated objects
fakeClient := fake.NewClientBuilder().
    WithScheme(scheme.Scheme).
    WithObjects(existingPod, existingNamespace).
    Build()

// Use it like a real client
err := fakeClient.Get(context.TODO(), key, &result)
```

---

### 9.3 Test getNodeNameForPVC (Optional Advanced Exercise)

**What to do:**

1. Create `TestGetNodeNameForPVC(t *testing.T)` in `transfer-pvc_test.go`
2. Import fake client: `"sigs.k8s.io/controller-runtime/pkg/client/fake"`
3. Create a fake pod that:
   - Has Status.Phase = Running
   - Has a Volume with PersistentVolumeClaim.ClaimName matching your test PVC
   - Has Spec.NodeName set to a test value

4. Create the fake client with this pod
5. Call `getNodeNameForPVC(fakeClient, namespace, pvcName)`
6. Verify it returns the expected node name

This is complex. If you're not comfortable yet, skip to the summary and return later.

---

## Part 10: Test Validation Methods

The `Validate()` methods check that required fields are set.

---

### 10.1 Test PvcFlags.Validate

**Open:** `cmd/transfer-pvc/transfer-pvc.go`, find `PvcFlags.Validate()`

```go
func (p *PvcFlags) Validate() error {
    if p.Name.source == "" {
        return fmt.Errorf("source pvc name cannot be empty")
    }
    // more checks...
    return nil
}
```

**What to do:**

1. Add `TestPvcFlags_Validate(t *testing.T)` to `transfer-pvc_test.go`
2. Test cases:
   - All fields valid: expect nil error
   - Empty source name: expect error containing "source pvc name"
   - Empty destination name: expect error containing "destination pvc name"
   - Empty source namespace: expect error
   - Empty destination namespace: expect error

3. To check error messages, use `strings.Contains`:
   ```go
   if err == nil || !strings.Contains(err.Error(), "expected text") {
       t.Errorf("expected error containing 'expected text'")
   }
   ```

4. Run: `go test ./cmd/transfer-pvc/... -run TestPvcFlags_Validate -v`

---

### 10.2 Test EndpointFlags.Validate

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

**What to do:**

1. Add `TestEndpointFlags_Validate(t *testing.T)`
2. Test cases:
   - Nginx type with subdomain: expect nil
   - Nginx type without subdomain: expect error
   - Route type: expect nil (no subdomain required)

3. Run: `go test ./cmd/transfer-pvc/... -run TestEndpointFlags_Validate -v`

---

## Summary

You learned:
- What PersistentVolumeClaims are (storage requests in Kubernetes)
- How multi-cluster operations work (contexts, clients)
- The Progress struct and its pointer fields
- The dataSize struct for representing byte amounts
- How rsync output is parsed
- Status state machine logic
- Testing functions that return errors
- Using subtests with `t.Run`
- (Optional) Using fake clients for functions that need Kubernetes

You wrote tests for:
- `parseRsyncLogs` (additional case)
- `Progress.Status()`
- `addDataSize`
- `newDataSize`
- `getValidatedResourceName`
- `PvcFlags.Validate()`
- `EndpointFlags.Validate()`
- (Optional) `getNodeNameForPVC`

---

## Next Steps

1. Run all transfer-pvc tests: `go test ./cmd/transfer-pvc/... -v`
2. Make sure they pass
3. Run all tests across the project: `go test ./... -v`

You've now covered unit tests for all three commands: apply, export, and transfer-pvc. You've learned:
- Go testing basics
- Table-driven tests
- Kubernetes concepts (resources, namespaces, RBAC, PVCs)
- Testing pure functions
- Handling pointers and nil values
- Error case testing
- (Optional) Mocking Kubernetes clients

Continue practicing by finding untested functions and writing tests for them. Each test deepens your understanding of both Go and Kubernetes.
