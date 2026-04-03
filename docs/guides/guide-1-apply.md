# Guide 1: Apply Command — Go Testing Foundations

You'll write unit tests for the `apply` command. This command transforms exported Kubernetes resources using transformation files. No Kubernetes knowledge needed yet — it's pure file operations.

By the end, you'll know how to:
- Structure a Go test file
- Write table-driven tests
- Use the `testing` package

---

## Part 1: Understand the Existing Test

Before writing tests, look at how this codebase already does it.

**Open this file:** `internal/file/file_helper_test.go`

Read it. Don't write anything yet. You'll notice patterns we'll use throughout.

---

### 1.1 The Test File Structure

**Why you need to know this:** Every Go test file follows the same structure. If you don't know it, you can't create one.

A Go test file has these parts:

1. **Package declaration** — Must be the same package as the code you're testing, OR the package name with `_test` suffix
2. **Imports** — The `testing` package, plus whatever packages your tests need
3. **Test functions** — Functions that start with `Test` and take `*testing.T` as parameter

The file `file_helper_test.go` uses `package file_test` (with `_test` suffix). This is called a "black-box test" — it tests the package from the outside, like a user would. It can only access exported (capitalized) functions.

---

### 1.2 The testing.T Parameter

**Why you need to know this:** Every test function receives this. It's how you report failures.

`*testing.T` is a pointer to a struct provided by Go's testing framework. The `*` means pointer — the function receives a reference to the struct, not a copy.

What you can do with `t`:
- `t.Errorf("message", args...)` — Report a failure but continue running the test
- `t.Fatalf("message", args...)` — Report a failure and stop this test immediately
- `t.Log("message")` — Print a message (only shows if test fails or you run with `-v`)

---

### 1.3 Table-Driven Tests

**Why you need to know this:** The existing tests use this pattern. You'll use it too.

A table-driven test runs the same logic with different inputs. Instead of writing 5 separate test functions, you define a "table" of test cases and loop through them.

The pattern has 3 parts:

1. **Define the table** — A slice of structs, where each struct is one test case
2. **Loop through cases** — `for _, test := range cases`
3. **Run the test logic** — Same logic for each case, using values from the struct

Look at `TestGetWhiteOutFilePath` in the file. It has:
- A slice called `cases`
- Each case has: `Name`, `Filepath`, `Dir`, `ResourceDir`, `Expected`
- A loop that tests each case

---

### 1.4 The Struct for Test Cases

**Why you need to know this:** You need to understand structs to read and write these tests.

In Go, a struct groups related values together. In the test file, you see:

```go
cases := []struct {
    Name        string
    Filepath    string
    Dir         string
    ResourceDir string
    Expected    string
}{
    {
        Name:        "test whiteout file creation",
        ...
    },
}
```

Breaking this down:
- `[]struct{...}` — A slice (dynamic array) of structs
- The struct has 5 fields, all strings
- `{...}` at the end — The actual values (one struct in this case)

You don't need to declare a named type. This is an "anonymous struct" — defined right where it's used.

---

## Part 2: Understand What You're Testing

Before testing code, you must understand what it does.

**Open this file:** `internal/file/file_helper.go`

---

### 2.1 What the Apply Command Does

**Why you need to know this:** You can't test code without understanding what problem it solves.

The apply command takes exported Kubernetes resources and transforms them. The process uses four directories:

1. **Export directory** — Contains the original exported resources (YAML files)
2. **Transform directory** — Contains transformation rules that modify resources
3. **Output directory** — Where the transformed resources get written
4. **Ignored patches directory** — Where skipped transformations get saved

---

### 2.2 What Are Ignored Patches?

**Why you need to know this:** One of the methods you'll test deals with ignored patches.

Sometimes a transformation produces changes you don't want to apply. Instead of silently discarding them, crane saves these unwanted changes to a separate directory called "ignored patches."

Why this matters:
- You can review what was skipped
- You can decide later if you actually want those changes
- Nothing is lost silently

---

### 2.3 What Are Whiteout Files?

**Why you need to know this:** Another method you'll test creates whiteout file paths.

A "whiteout" file is a marker that tells crane to skip a resource entirely. If you have a resource in your export directory but you don't want it in the output, you create a whiteout file for it.

The name comes from Unix/Docker, where whiteout files mark deleted files in layered filesystems.

---

### 2.4 The PathOpts Struct

**Why you need to know this:** This is the main thing you'll test. You need to know what it holds.

`PathOpts` stores the four directory paths:

```go
type PathOpts struct {
    TransformDir      string  // Where transformation rules live
    ExportDir         string  // Where original exported resources live
    OutputDir         string  // Where transformed resources get written
    IgnoredPatchesDir string  // Where skipped transformations get saved
}
```

Each field is a `string` holding a file path.

---

### 2.5 The Methods on PathOpts

**Why you need to know this:** These are the functions you'll test.

In Go, a method is a function attached to a type. The `(opts *PathOpts)` part is called the "receiver" — it's like `this` or `self` in other languages.

The methods you'll test:

1. `GetWhiteOutFilePath(filePath string) string`
   - Takes an export file path
   - Returns the path where a whiteout marker would be (to skip this resource)
   - Adds `.wh.` prefix to the filename

2. `GetTransformPath(filePath string) string`
   - Takes an export file path
   - Returns where the transformation rules for this resource would be
   - Adds `transform-` prefix to the filename

3. `GetOutputFilePath(filePath string) string`
   - Takes an export file path
   - Returns where to write the transformed resource
   - No prefix, just changes the directory from export to output

4. `GetIgnoredPatchesPath(filePath string) string`
   - Takes an export file path
   - Returns where skipped transformations for this resource would be saved
   - Adds `ignored-` prefix to the filename

---

### 2.6 How These Methods Work

**Why you need to know this:** To test something, you need to know what the correct output should be.

All these methods do similar work:
1. Take an input path like `/export/ns/myfile.yaml`
2. Replace the base directory (`ExportDir`) with another directory (`TransformDir`, `OutputDir`, etc.)
3. Optionally add a prefix to the filename

Example:
- Input: `/fully/qualified/resources/ns/configmap.yaml`
- ExportDir: `/fully/qualified/resources`
- TransformDir: `/fully/qualified/transform`
- Output of `GetTransformPath`: `/fully/qualified/transform/ns/transform-configmap.yaml`

The directory part changes. The filename gets a prefix.

---

## Part 3: Write Your First Test

Now you write. You'll add more test cases to the existing tests.

---

### 3.1 Add a Test Case to TestGetWhiteOutFilePath

**The task:** The existing test has only one case. Add a second case that tests a different scenario.

**What to do:**

1. Open `internal/file/file_helper_test.go`
2. Find the `TestGetWhiteOutFilePath` function
3. Inside the `cases` slice, add a second struct after the first one
4. Use these values:
   - Name: describe what this case tests (e.g., "nested subdirectory")
   - Filepath: a path with deeper nesting, like `/base/resources/ns/subdir/myfile.yaml`
   - Dir: where transforms live, like `/base/transform`
   - ResourceDir: the export dir, like `/base/resources`
   - Expected: what the output should be — figure this out by understanding how `updatePath` works in `file_helper.go`

5. Run the test to verify it passes: `go test ./internal/file/...`

---

### 3.2 Add Test Cases for Edge Cases

**The task:** Good tests check edge cases — unusual inputs that might break things.

Think about what could go wrong:
- What if the filename has no extension?
- What if the path has spaces?
- What if the ResourceDir doesn't match the Filepath?

Add test cases for at least 2 edge cases. For each:
1. Figure out what the expected output should be
2. Add the case to the table
3. Run tests to verify

---

## Part 4: Test a Method Without Existing Tests

You learned in Section 2.2 that ignored patches are skipped transformations saved for review. The method `GetIgnoredPatchesPath` generates the path where these would be saved. It has no tests — you'll write them.

---

### 4.1 Understand the Method First

Look at `GetIgnoredPatchesPath` in `file_helper.go`:

```go
func (opts *PathOpts) GetIgnoredPatchesPath(filePath string) string {
    return opts.updateIgnoredPatchesDirPath("ignored-", filePath)
}
```

Now look at `updateIgnoredPatchesDirPath`:

```go
func (opts *PathOpts) updateIgnoredPatchesDirPath(prefix, filePath string) string {
    if len(opts.IgnoredPatchesDir) == 0 {
        return ""
    }
    return opts.updatePath(opts.IgnoredPatchesDir, prefix, filePath)
}
```

Notice something: if `IgnoredPatchesDir` is empty, it returns empty string. This is a case you need to test.

---

### 4.2 Write the Test Function

**What to do:**

1. In `file_helper_test.go`, create a new function called `TestGetIgnoredPatchesPath`
2. It takes `t *testing.T` as parameter
3. Define a `cases` slice with a struct that has these fields:
   - `Name` (string) — description of the test case
   - `Filepath` (string) — the input path
   - `IgnoredPatchesDir` (string) — the ignored patches directory
   - `ExportDir` (string) — the export directory
   - `Expected` (string) — what the output should be

4. Add at least 3 test cases:
   - Normal case: all fields populated, expect prefixed path
   - Empty IgnoredPatchesDir: expect empty string
   - Nested path: deeper directory structure

5. Write the loop that:
   - Creates a `PathOpts` with the values from each case
   - Calls `opts.GetIgnoredPatchesPath(test.Filepath)`
   - Compares result to expected using `t.Errorf` if they don't match

6. Run: `go test ./internal/file/...`

---

## Part 5: The File Struct

Now you understand `PathOpts`. Next, understand the `File` struct — you'll need it for testing the apply command itself.

---

### 5.1 What File Holds

**Why you need to know this:** The apply command works with `File` structs.

```go
type File struct {
    Info         os.FileInfo
    Unstructured unstructured.Unstructured
    Path         string
}
```

Three fields:
- `Info` — Metadata about the file (name, size, permissions). This is a Go standard library type.
- `Unstructured` — The parsed content of the file. This is a Kubernetes type — more on this below.
- `Path` — The full path to the file as a string.

---

### 5.2 What is unstructured.Unstructured?

**Why you need to know this:** Every Kubernetes resource in crane is stored as this type.

Kubernetes resources (like Deployments, Services, ConfigMaps) are structured data — they have specific fields. But there are hundreds of resource types, and they keep adding more.

Instead of having a Go struct for every possible type, Kubernetes provides `unstructured.Unstructured`. It can hold any resource as a map of key-value pairs.

Think of it like JSON stored in a Go map:

```go
// Conceptually, an Unstructured is like:
map[string]interface{}{
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": map[string]interface{}{
        "name": "my-config",
        "namespace": "default",
    },
    "data": map[string]interface{}{
        "key": "value",
    },
}
```

You can get values using methods like:
- `u.GetName()` — Returns the resource name
- `u.GetNamespace()` — Returns the namespace
- `u.GetKind()` — Returns the kind (ConfigMap, Deployment, etc.)

---

### 5.3 Why This Matters for Testing

When testing the apply command, you'll need to create `File` structs with mock data. You now know:
- `Path` is just a string
- `Info` can be mocked (we'll cover this later)
- `Unstructured` needs to be populated with valid resource data

---

## Part 6: Testing the Apply Command

The apply command is in `cmd/apply/apply.go`. The main logic is in the `run()` method.

---

### 6.1 Why run() is Hard to Test Directly

Look at `run()`. It:
- Reads from the filesystem
- Calls external functions (`file.ReadFiles`, `apply.Applier{}`)
- Writes to the filesystem

Functions that do I/O are hard to unit test. You'd need real files or complex mocking.

For now, you've tested the helper functions. In Guide 2, you'll learn techniques for testing functions with more dependencies.

---

## Summary

You learned:
- Go test file structure (package, imports, Test functions)
- The `*testing.T` parameter and `t.Errorf`
- Table-driven tests (cases slice, loop, check)
- The `PathOpts` struct and its methods
- The `File` struct and `unstructured.Unstructured`

You wrote:
- Additional test cases for existing tests
- A new test function for `GetIgnoredPatchesPath`

---

## Next Steps

When you've completed all tasks in this guide:
1. Run all tests: `go test ./internal/file/... -v`
2. Make sure they all pass
3. Move to Guide 2: Export Command

In Guide 2, you'll learn Kubernetes concepts and how to mock dependencies for testing more complex functions.
