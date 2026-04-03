# Guide 2: Export Command — Kubernetes Concepts Through Testing

You'll write unit tests for the `export` command. This command connects to a Kubernetes cluster and exports resources to files.

By the end, you'll know:
- What Kubernetes resources are
- What namespaces, groups, and kinds mean
- How to test functions that use Kubernetes types
- How to mock Kubernetes clients

---

## Part 1: What is Kubernetes?

**Why you need to know this:** The export command exports Kubernetes resources. You can't test it without understanding what it's exporting.

---

### 1.1 Kubernetes in One Paragraph

You write an application. You want to run it on servers. But you don't want to manually install it on each server, restart it when it crashes, or figure out which server has space.

Kubernetes does this for you. You tell it "run my application" and it picks servers, starts your app, restarts it if it crashes, and spreads copies across machines. You describe what you want in YAML files, Kubernetes makes it happen.

---

### 1.2 Containers First

**Why you need to know this:** Kubernetes runs containers. You need to know what a container is before understanding what Kubernetes manages.

A container is your application packaged with everything it needs to run — code, libraries, settings. It's like a lightweight virtual machine, but faster to start and smaller.

You build a container image (a snapshot of your app), then run copies of it. Each running copy is a container.

---

### 1.3 Resources

**Why you need to know this:** The export command exports "resources." Every function you'll test deals with resources.

A "resource" is anything Kubernetes manages. You create resources by writing YAML files. Kubernetes reads them and does what they describe.

All resources are defined as YAML files. They all look similar — fields like `apiVersion`, `kind`, `metadata`. What makes each resource type different is its **purpose** and the **specific fields** it has.

Common resources you'll see in this codebase:

**Pod** — Runs one or more containers together on the same machine, sharing network and storage.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  namespace: default
spec:
  containers:
    - name: web
      image: nginx:latest
```

The `spec:` section describes what to run. Here: one container named `web` using the `nginx` image.

**Deployment** — Tells Kubernetes how to run your app: which Pod to run, how many copies, and what to do if one crashes.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: web
          image: nginx:latest
```

The `replicas: 3` means "run 3 copies." The `template:` section describes the Pod to run (same structure as above).

**ConfigMap** — Holds configuration data that your app reads at runtime.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: default
data:
  database_url: "postgres://localhost:5432"
```

The `data:` section is where you put your configuration. Your app reads values from here instead of hardcoding them.

**Secret** — Same purpose as ConfigMap, but for sensitive data like passwords. Kubernetes stores it encoded.

**Service** — Gives you one stable network address that routes to your Pods. Pods come and go (crash, get replaced), each with a new address. Service stays the same.

---

### 1.4 The Key Fields Every Resource Has

**Why you need to know this:** The code you'll test reads these fields.

Look at the ConfigMap example above. Every resource has:

- **apiVersion** — Which version of the API defines this resource (`v1` in the example)
- **kind** — What type of resource this is (`ConfigMap` in the example)
- **metadata** — Information about the resource:
  - **name** — The resource's name (`my-config`)
  - **namespace** — Which namespace it belongs to (`default`)

**Example: Multiple Pods sharing a ConfigMap**

If multiple Pods need the same database URL, you create one ConfigMap and have all Pods read from it:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: db-config
data:
  database_url: "postgres://localhost:5432"
```

Then reference it in a Deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: web
        image: my-node-app
        envFrom:
        - configMapRef:
            name: db-config
```

All 3 Pods get `database_url` as an environment variable. If the URL changes, you update the ConfigMap once.

---

### 1.5 Namespaces

**Why you need to know this:** The export command exports resources from a specific namespace.

A namespace is a way to organize resources. Think of it like folders:
- `/home/alice/` — Alice's files
- `/home/bob/` — Bob's files

Alice and Bob can both have a file named `config.txt` because they're in different folders.

Same in Kubernetes:
- `default` namespace — Used if you don't specify one
- `kube-system` namespace — Kubernetes internal components
- `my-app` namespace — Your application's resources

A ConfigMap named `config` can exist in both `default` and `my-app` namespaces — they're separate resources.

**Cluster-scoped resources:** Some resources don't belong to any namespace. They apply to the whole cluster. Example: Nodes (the physical/virtual servers themselves). You'll learn about others when you need them.

---

### 1.6 API Groups and Versions

**Why you need to know this:** The code checks API groups to decide what to export.

Resources are organized into groups:
- **Core group** — Basic resources: Pods, Services, ConfigMaps, Secrets. The apiVersion is just `v1` (no group prefix).
- **apps group** — Resources for running applications: Deployments. The apiVersion is `apps/v1`.

The `apiVersion` field shows the group and version:
- `v1` — Core group, version 1
- `apps/v1` — Apps group, version 1

There are more groups, but you'll learn them when functions you test use them.

---

## Part 2: The Data Structures in Export

Open `cmd/export/discover.go`. You'll see custom types used to track resources.

---

### 2.1 The groupResource Struct

**Why you need to know this:** This struct holds the resources being exported. You'll create these in tests.

```go
type groupResource struct {
    APIGroup        string
    APIVersion      string
    APIGroupVersion string
    APIResource     metav1.APIResource
    objects         *unstructured.UnstructuredList
}
```

Let's break down each field:

- **APIGroup** — The group name (e.g., `"apps"`, `"rbac.authorization.k8s.io"`, or `""` for core resources)
- **APIVersion** — The version (e.g., `"v1"`, `"v1beta1"`)
- **APIGroupVersion** — Group and version combined (e.g., `"apps/v1"`, `"v1"`)
- **APIResource** — Metadata about the resource type (explained below)
- **objects** — A list of actual resources of this type

---

### 2.2 The metav1.APIResource Type

**Why you need to know this:** It's a field in `groupResource`. You need to populate it in tests.

`metav1.APIResource` comes from the Kubernetes client library. It describes a resource type (not a specific resource, but the type itself).

Key fields you'll use:
```go
type APIResource struct {
    Name       string   // Lowercase plural name (e.g., "configmaps", "deployments")
    Kind       string   // Singular kind name (e.g., "ConfigMap", "Deployment")
    Namespaced bool     // True if resources of this type live in namespaces
    Verbs      []string // What you can do: "get", "list", "create", "delete", etc.
}
```

Example:
```go
resource := metav1.APIResource{
    Name:       "configmaps",
    Kind:       "ConfigMap",
    Namespaced: true,
    Verbs:      []string{"get", "list", "create", "delete"},
}
```

---

### 2.3 The unstructured.UnstructuredList Type

**Why you need to know this:** It holds multiple resources. You learned `Unstructured` in Guide 1 — this is a list of them.

```go
type UnstructuredList struct {
    Items []Unstructured  // Slice of individual resources
    // ... other fields
}
```

To create one with items:
```go
list := &unstructured.UnstructuredList{
    Items: []unstructured.Unstructured{
        // individual resources go here
    },
}
```

---

### 2.4 The groupResourceError Struct

**Why you need to know this:** When export fails for a resource type, this struct records the error.

```go
type groupResourceError struct {
    APIResource metav1.APIResource `json:",inline"`
    Error       error              `json:"error"`
}
```

Two fields:
- **APIResource** — Which resource type had the error
- **Error** — What went wrong

---

## Part 3: Test the Simplest Functions First

Start with functions that have no dependencies — they take input, return output, nothing else.

---

### 3.1 Test getFilePath

**Open:** `cmd/export/discover.go`, find `getFilePath`

```go
func getFilePath(obj unstructured.Unstructured) string {
    namespace := obj.GetNamespace()
    if namespace == "" {
        namespace = "clusterscoped"
    }
    return strings.Join([]string{
        obj.GetKind(),
        obj.GetObjectKind().GroupVersionKind().GroupKind().Group,
        obj.GetObjectKind().GroupVersionKind().Version,
        namespace,
        obj.GetName(),
    }, "_") + ".yaml"
}
```

This function creates a filename from a resource. Let's understand it:

1. Gets the namespace (or "clusterscoped" if none)
2. Joins these with underscores: Kind, Group, Version, Namespace, Name
3. Adds ".yaml" extension

Example outputs:
- ConfigMap (apiVersion: `v1`): `ConfigMap__v1_default_my-config.yaml` — double underscore because Group is empty for core resources
- Deployment (apiVersion: `apps/v1`): `Deployment_apps_v1_default_my-deploy.yaml` — Group is `apps`

---

### 3.2 Create a Test File

**What to do:**

1. Create a new file: `cmd/export/discover_test.go`
2. Add package declaration: `package export`
3. Import what you'll need:
   - `"testing"` — For the test framework
   - `"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"` — For creating Unstructured objects

---

### 3.3 Create an Unstructured Object for Testing

**Why you need to know this:** You need to pass an `Unstructured` to `getFilePath`. You need to know how to make one.

An `Unstructured` wraps a `map[string]interface{}`. You can set it up like this:

```go
obj := unstructured.Unstructured{}
obj.SetKind("ConfigMap")
obj.SetAPIVersion("v1")
obj.SetNamespace("default")
obj.SetName("my-config")
```

The `Set` methods modify the underlying map. The `Get` methods read from it.

---

### 3.4 Write the Test

**What to do:**

1. Create a function `TestGetFilePath(t *testing.T)`
2. Define a table of test cases with these fields:
   - `name` (string) — Description
   - `kind` (string) — The resource kind
   - `apiVersion` (string) — The API version
   - `namespace` (string) — The namespace (empty string for cluster-scoped)
   - `resourceName` (string) — The resource name
   - `expected` (string) — The expected filename

3. Add test cases:
   - Namespaced resource: ConfigMap in "default" namespace
   - Cluster-scoped resource: empty namespace (should become "clusterscoped")
   - Resource with group: kind "Deployment", apiVersion "apps/v1"

4. In the loop:
   - Create an `Unstructured` object
   - Set kind, apiVersion, namespace, name using the Set methods
   - Call `getFilePath(obj)`
   - Compare to expected using `t.Errorf` if different

5. Run: `go test ./cmd/export/... -run TestGetFilePath -v`

---

### 3.5 Understanding the Group in the Filename

**Why you need to know this:** The filename includes the group. You need to understand how to extract it.

Look at this part of `getFilePath`:
```go
obj.GetObjectKind().GroupVersionKind().GroupKind().Group
```

This chain:
1. `GetObjectKind()` — Returns something that has group/version/kind info
2. `GroupVersionKind()` — Returns a `GroupVersionKind` struct
3. `GroupKind()` — Returns a `GroupKind` struct (just group and kind)
4. `Group` — The group string

For `apiVersion: v1` (core resources), the group is empty string `""`.
For `apiVersion: apps/v1`, the group is `"apps"`.

When you set `obj.SetAPIVersion("apps/v1")`, internally it parses this and stores the group.

---

## Part 4: Test isClusterScopedResource

This function checks if a resource kind is in the allowed list for cluster-scoped export.

---

### 4.1 Cluster-Scoped Resources

When you export a namespace, you get all the resources in it — ConfigMaps, Deployments, Secrets, etc. These are "namespaced" resources.

But some resources don't belong to any namespace. They're "cluster-scoped" — they apply to the whole cluster. You learned this in Part 1.

Example of cluster-scoped resources:
- **ClusterRole** — Defines what actions are allowed (like "can read all ConfigMaps in the cluster")
- **ClusterRoleBinding** — Says who has that ClusterRole (like "the `my-app` ServiceAccount has the `reader` ClusterRole")

Together, ClusterRole and ClusterRoleBinding control permissions. This is called **RBAC** (Role-Based Access Control).

---

### 4.2 Why Crane Cares About Cluster-Scoped Resources

When you migrate an app to a new cluster, you need its permissions too. If your app has a ClusterRoleBinding that grants it permission to read secrets, you want that exported.

But you don't want ALL cluster-scoped resources — just the ones related to your namespace. So crane has three controls:

**1. An allowed list of resource kinds**

Even with the flag on, crane doesn't export every cluster-scoped resource. There's a hardcoded list in the code that says which kinds are allowed: ClusterRole, ClusterRoleBinding, and SecurityContextConstraints. If a resource is cluster-scoped but not in this list (like a Node), it gets skipped.

**2. Filtering by namespace relevance**

Even if a ClusterRoleBinding passes the first two checks, crane only exports it if it's actually relevant to the namespace being exported. You'll test this logic later — it involves checking which ServiceAccounts the binding references.

**3. A command-line flag: `--cluster-scoped-rbac`** (more in Part 5)

When the user runs `crane export`, they can pass this flag. If they don't pass it (or set it to false), crane skips all cluster-scoped resources — it only exports namespaced ones. If they pass it, crane considers exporting cluster-scoped RBAC resources.

---

### 4.3 The admittedResource Struct

**Why you need to know this:** The function uses this struct to store allowed resource types.

```go
type admittedResource struct {
    APIgroup string
    Kind     string
}
```

Two fields:
- **APIgroup** — The API group (e.g., `"rbac.authorization.k8s.io"`)
- **Kind** — The resource kind (e.g., `"ClusterRole"`)

---

### 4.4 Understand the Function

**Open:** `cmd/export/cluster.go`

```go
var admittedClusterScopeResources = []admittedResource{
    {Kind: "ClusterRoleBinding", APIgroup: "rbac.authorization.k8s.io"},
    {Kind: "ClusterRole", APIgroup: "rbac.authorization.k8s.io"},
    {Kind: "SecurityContextConstraints", APIgroup: "security.openshift.io"},
}

func isClusterScopedResource(apiGroup string, kind string) bool {
    for _, admitted := range admittedClusterScopeResources {
        if admitted.Kind == kind && admitted.APIgroup == apiGroup {
            return true
        }
    }
    return false
}
```

This is a simple lookup: does (apiGroup, kind) exist in the admitted list?

---

### 4.5 Write the Test

**What to do:**

1. Create a new test file: `cmd/export/cluster_test.go`
2. Package: `package export`
3. Add `TestIsClusterScopedResource(t *testing.T)`
4. Test cases:
   - ClusterRole with correct group: expect true
   - ClusterRoleBinding with correct group: expect true
   - SecurityContextConstraints with correct group: expect true
   - ClusterRole with wrong group: expect false
   - Random kind with random group: expect false
   - Node (real K8s kind but not in list): expect false

5. Run: `go test ./cmd/export/... -run TestIsClusterScopedResource -v`

---

## Part 5: Test isAdmittedResource

This function decides whether a resource should be exported.

---

### 5.1 Understand the Function

**Why you need to know this:** This is the function you'll test. It implements controls #1 (the flag) and #2 (the allowed list) from Part 4, section 4.2.

The function takes three parameters:
- `clusterScopedRbac` — the flag value (true if user passed `--cluster-scoped-rbac`)
- `gv` — the resource's API group and version
- `resource` — metadata about the resource type, including whether it's namespaced

```go
func isAdmittedResource(clusterScopedRbac bool, gv schema.GroupVersion, resource metav1.APIResource) bool {
    if !resource.Namespaced {
        return clusterScopedRbac && isClusterScopedResource(gv.Group, resource.Kind)
    }
    return true
}
```

Reading the code:
- `if !resource.Namespaced` — if the resource is cluster-scoped (NOT namespaced)...
- `return clusterScopedRbac && isClusterScopedResource(...)` — return true only if BOTH: the flag is on, AND `isClusterScopedResource` returns true (meaning it's in the hardcoded allowed list)
- `return true` — if we get here, the resource is namespaced, so always admit it

---

### 5.2 The schema.GroupVersion Type

**Why you need to know this:** It's a parameter to `isAdmittedResource`.

```go
type GroupVersion struct {
    Group   string  // e.g., "apps", "rbac.authorization.k8s.io", or ""
    Version string  // e.g., "v1", "v1beta1"
}
```

You'll need to import `"k8s.io/apimachinery/pkg/runtime/schema"` to use it.

Create one like this:
```go
gv := schema.GroupVersion{Group: "apps", Version: "v1"}
```

---

### 5.3 Understanding Resource Properties

**Why you need to know this:** To write test cases, you need to know that kind, group, and namespaced aren't independent — they're fixed properties of real resource types.

A ConfigMap is always:
- kind: `ConfigMap`
- group: `""` (core group)
- namespaced: `true`

You can't have a ConfigMap with namespaced=false — that's not how ConfigMaps work.

A ClusterRole is always:
- kind: `ClusterRole`
- group: `rbac.authorization.k8s.io`
- namespaced: `false`

You can't have a ClusterRole with namespaced=true — ClusterRoles are always cluster-scoped.

When you write test cases, pick a real resource type and use its real properties.

---

### 5.4 Walk Through One Test Case

Let's think through a test case together.

**Scenario:** We're exporting a ConfigMap. The `clusterScopedRbac` flag is false.

Think about it:
1. Is ConfigMap namespaced? Yes.
2. Look at the function — if `resource.Namespaced` is true, what does it return? It returns `true` immediately.
3. The `clusterScopedRbac` flag doesn't matter for namespaced resources.

**Expected result:** `true`

**Test case values:**
- kind: `ConfigMap`
- group: `""` (ConfigMaps are in core group)
- version: `v1`
- namespaced: `true` (ConfigMaps live in namespaces)
- clusterScopedRbac: `false`
- expected: `true`

---

### 5.5 Write the First Test Case

**What to do:**

1. Create `TestIsAdmittedResource(t *testing.T)` in your test file
2. Import `"k8s.io/apimachinery/pkg/runtime/schema"` and `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`
3. Define a test case struct with fields: `name`, `clusterScopedRbac`, `group`, `version`, `kind`, `namespaced`, `expected`
4. Add the ConfigMap test case from above
5. In the loop:
   - Create `schema.GroupVersion` using group and version
   - Create `metav1.APIResource` with Kind and Namespaced fields
   - Call `isAdmittedResource(test.clusterScopedRbac, gv, resource)`
   - Compare result to expected
6. Run: `go test ./cmd/export/... -run TestIsAdmittedResource -v`

---

### 5.6 Add More Test Cases

Now add test cases for these scenarios. For each, think through the logic like we did above:

1. **ClusterRole with clusterScopedRbac=true** — ClusterRole is cluster-scoped, in the allowed list. What should happen?

2. **ClusterRole with clusterScopedRbac=false** — Same resource, but the flag is off. What should happen?

3. **Node with clusterScopedRbac=true** — Node is cluster-scoped (group: `""`, namespaced: false), but it's NOT in the allowed list. What should happen?

**Node properties:**

A Node represents a server (physical or virtual machine) in the cluster. Kubernetes tracks which nodes are available to run containers. Nodes are cluster-scoped — they don't belong to any namespace.

- kind: `Node`
- group: `""` (core group)
- namespaced: `false`
- In the allowed list: no

Run your tests after adding each case.

---

## Part 6: Test anyServiceAccountInNamespace

This function checks if a ServiceAccount exists in the list.

---

### 6.1 What is a ServiceAccount?

**Why you need to know this:** The function you'll test works with ServiceAccounts.

A **ServiceAccount** is an identity for processes running inside the cluster. Instead of a username/password, pods use ServiceAccounts to authenticate to Kubernetes.

Each namespace has a `default` ServiceAccount. You can create more for specific purposes.

When crane exports a namespace, it finds which ClusterRoleBindings reference the ServiceAccounts in that namespace, and exports those too.

---

### 6.2 Understand the Context

**Why this function exists:** When exporting cluster-scoped RBAC (permissions), crane only exports ClusterRoleBindings that reference ServiceAccounts being exported. This function checks if a ServiceAccount is in the export list.

---

### 6.3 The ClusterScopedRbacHandler Struct

**Why you need to know this:** The function you'll test is a method on this struct. You need to understand it to create test instances.

**What this struct does:** When exporting with `--cluster-scoped-rbac`, crane needs to filter ClusterRoleBindings to only include ones that reference ServiceAccounts from the namespace being exported. This struct handles that filtering. It holds the list of ServiceAccounts being exported and filters cluster-scoped resources accordingly.

```go
type ClusterScopedRbacHandler struct {
    log              logrus.FieldLogger
    readyToFilter    bool
    serviceAccounts  []unstructured.Unstructured
    clusterResources map[string]*groupResource
    filteredClusterRoleBindings *groupResource
}
```

Five fields:
- **log** — A logger for debug output. You'll learn how to create one in the next section.
- **readyToFilter** — Tracks whether the handler is ready to filter resources. Not used by the method you'll test.
- **serviceAccounts** — A slice of ServiceAccounts being exported. This is what the method searches through.
- **clusterResources** — A map of cluster-scoped resources by kind. Not used by the method you'll test.
- **filteredClusterRoleBindings** — The ClusterRoleBindings that passed filtering. Not used by the method you'll test.

For testing `anyServiceAccountInNamespace`, you only need to set `log` and `serviceAccounts`.

---

### 6.4 The Function

**Why you need to know this:** This is the function you'll test.

```go
func (c *ClusterScopedRbacHandler) anyServiceAccountInNamespace(namespaceName string, serviceAccountName string) bool {
    c.log.Debugf("Looking for SA %s in %s", serviceAccountName, namespaceName)
    for _, sa := range c.serviceAccounts {
        if sa.GetName() == serviceAccountName && sa.GetNamespace() == namespaceName {
            return true
        }
    }
    return false
}
```

The method:
- Takes namespace and name
- Loops through `c.serviceAccounts`
- Returns true if any matches both name and namespace

---

### 6.5 Creating a Test Logger

**Why you need to know this:** The struct has a `log` field. You need to provide one.

The `logrus.FieldLogger` is an interface. Logrus is a logging library. To create a logger for tests:

```go
import "github.com/sirupsen/logrus"

log := logrus.New()
// optionally silence it:
log.SetOutput(io.Discard)
```

You'll need to import `"io"` for `io.Discard`.

---

### 6.6 Write the Test

**What to do:**

1. In `cluster_test.go`, add `TestAnyServiceAccountInNamespace(t *testing.T)`
2. Import `"github.com/sirupsen/logrus"`, `"io"`, and `"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"`
3. Create a logger: `log := logrus.New()` and `log.SetOutput(io.Discard)`
4. Create a `ClusterScopedRbacHandler` with some ServiceAccounts in the list:
   - Create 2-3 `Unstructured` objects representing ServiceAccounts
   - Set their name and namespace
   - Put them in the handler's `serviceAccounts` slice

5. Test cases:
   - SA exists with matching name and namespace: expect true
   - SA exists but different namespace: expect false
   - SA exists but different name: expect false
   - No matching SA: expect false

6. Run: `go test ./cmd/export/... -run TestAnyServiceAccountInNamespace -v`

---

## Part 7: Mocking Kubernetes Clients (Advanced)

The functions `getObjects` and `resourceToExtract` talk to a real Kubernetes cluster. To test them, you need to mock the client.

---

### 7.1 What is dynamic.Interface?

Look at the signature:
```go
func getObjects(g *groupResource, namespace string, labelSelector string, d dynamic.Interface, ...) ...
```

`dynamic.Interface` is a Kubernetes client interface. It can fetch any resource type (that's why it's "dynamic" — not tied to specific types).

An interface in Go defines methods that a type must have. `dynamic.Interface` defines methods like:
- `Resource(gvr schema.GroupVersionResource)` — Returns a client for a specific resource type

---

### 7.2 Why Mocking is Hard Here

The dynamic client is complex. It returns another interface which returns another interface. Mocking all these layers is tedious.

Approaches:
1. **Skip these tests** — Focus on pure functions (what you've done so far)
2. **Use fake clients** — Kubernetes provides `fake.NewSimpleDynamicClient`
3. **Integration tests** — Run against a real cluster (out of scope for unit tests)

---

### 7.3 Using the Fake Client (Optional Advanced Exercise)

If you want to try:

1. Import `"k8s.io/client-go/dynamic/fake"` and `"k8s.io/apimachinery/pkg/runtime"`
2. Create a scheme: `scheme := runtime.NewScheme()`
3. Create objects to pre-populate the fake client
4. Create the client: `client := fake.NewSimpleDynamicClient(scheme, objects...)`
5. Pass it to `getObjects`

This is complex. For now, the pure function tests give you good coverage. Return to this after you're comfortable with the basics.

---

## Summary

You learned:
- What Kubernetes resources are (kind, apiVersion, namespace, name)
- API groups and versions
- Namespaces and cluster-scoped resources
- RBAC basics (Roles, Bindings, ServiceAccounts)
- The data structures: `groupResource`, `metav1.APIResource`, `schema.GroupVersion`
- How to create `Unstructured` objects for testing
- How to test pure functions in the export command

You wrote tests for:
- `getFilePath`
- `isAdmittedResource`
- `isClusterScopedResource`
- `anyServiceAccountInNamespace`

---

## Next Steps

1. Run all export tests: `go test ./cmd/export/... -v`
2. Make sure they pass
3. Move to Guide 3: Transfer-PVC Command

Guide 3 covers the most complex command — multi-cluster operations and complex state management.
