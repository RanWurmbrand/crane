package framework

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/gomega"
)

type Deletable interface {
	Delete(k KubectlRunner) error
}

func ResourceCleanup(clusters []KubectlRunner, resources []Deletable) {
	for _, k := range clusters {
		for _, r := range resources {
			if err := r.Delete(k); err != nil {
				log.Printf("cleanup: %v", err)
			}
		}
	}
}

type ClusterRole struct {
	Name       string
	Permission string
	Label      string
}

func (cr ClusterRole) Create(k KubectlRunner) error {
	var verbs string
	switch cr.Permission {
	case "write":
		verbs = "get,list,watch,create,update,delete"
	case "patch":
		verbs = "get,list,watch,patch"
	default:
		verbs = "get,list,watch"
	}
	_, err := k.Run("create", "clusterrole", cr.Name, "--verb="+verbs, "--resource=pods")
	if err != nil {
		log.Printf("failed to create ClusterRole %s: %v", cr.Name, err)
		return err
	}
	log.Printf("created %s ClusterRole %s", cr.Permission, cr.Name)
	if cr.Label != "" {
		_, err = k.Run("label", "clusterrole", cr.Name, cr.Label)
		if err != nil {
			return fmt.Errorf("failed to label ClusterRole %s: %w", cr.Name, err)
		}
	}
	return nil
}

func (cr ClusterRole) Delete(k KubectlRunner) error {
	_, err := k.Run("delete", "clusterrole", cr.Name, "--ignore-not-found=true")
	if err != nil {
		return fmt.Errorf("failed to delete ClusterRole %s: %w", cr.Name, err)
	}
	return nil
}

type ClusterRoleBinding struct {
	Name            string
	ClusterRoleName string
	Subject         string
	Label           string
}

func (crb ClusterRoleBinding) Create(k KubectlRunner) error {
	_, err := k.Run("create", "clusterrolebinding", crb.Name, "--clusterrole="+crb.ClusterRoleName, crb.Subject)
	if err != nil {
		log.Printf("failed to create ClusterRoleBinding %s: %v", crb.Name, err)
		return err
	}
	log.Printf("created ClusterRoleBinding %s -> ClusterRole %s", crb.Name, crb.ClusterRoleName)
	if crb.Label != "" {
		_, err = k.Run("label", "clusterrolebinding", crb.Name, crb.Label)
		if err != nil {
			return fmt.Errorf("failed to label ClusterRoleBinding %s: %w", crb.Name, err)
		}
	}
	return nil
}

func (crb ClusterRoleBinding) Delete(k KubectlRunner) error {
	_, err := k.Run("delete", "clusterrolebinding", crb.Name, "--ignore-not-found=true")
	if err != nil {
		return fmt.Errorf("failed to delete ClusterRoleBinding %s: %w", crb.Name, err)
	}
	return nil
}

type ServiceAccount struct {
	Name      string
	Namespace string
	Label     string
}

func (sa ServiceAccount) Create(k KubectlRunner) error {
	_, err := k.Run("create", "serviceaccount", sa.Name, "-n", sa.Namespace)
	if err != nil {
		log.Printf("failed to create ServiceAccount %s in %s: %v", sa.Name, sa.Namespace, err)
		return err
	}
	log.Printf("created ServiceAccount %s in %s", sa.Name, sa.Namespace)
	if sa.Label != "" {
		_, err = k.Run("label", "serviceaccount", sa.Name, "-n", sa.Namespace, sa.Label)
		if err != nil {
			return fmt.Errorf("failed to label ServiceAccount %s: %w", sa.Name, err)
		}
	}
	return nil
}

func (sa ServiceAccount) Delete(k KubectlRunner) error {
	_, err := k.Run("delete", "serviceaccount", sa.Name, "-n", sa.Namespace, "--ignore-not-found=true")
	if err != nil {
		return fmt.Errorf("failed to delete ServiceAccount %s: %w", sa.Name, err)
	}
	return nil
}

type CustomResourceDefinition struct {
	Name string
	YAML string
}

func (crd CustomResourceDefinition) Create(k KubectlRunner) error {
	_, err := k.RunWithStdin(crd.YAML, "apply", "-f", "-")
	if err != nil {
		log.Printf("failed to create CRD %s: %v", crd.Name, err)
	} else {
		log.Printf("created CRD %s", crd.Name)
	}
	return err
}

func (crd CustomResourceDefinition) Delete(k KubectlRunner) error {
	_, err := k.Run("delete", "crd", crd.Name, "--ignore-not-found=true")
	if err != nil {
		return fmt.Errorf("failed to delete CRD %s: %w", crd.Name, err)
	}
	return nil
}

func (crd CustomResourceDefinition) WaitForEstablished(k KubectlRunner) error {
	gomega.Eventually(func() (string, error) {
		return k.Run("get", "crd", crd.Name,
			"-o", "jsonpath={.status.conditions[?(@.type=='Established')].status}")
	}, "30s", "2s").Should(gomega.Equal("True"))
	log.Printf("CRD %s is Established", crd.Name)
	return nil
}

type CustomResource struct {
	Name      string
	Namespace string
	Kind      string
	YAML      string
}

func (cr CustomResource) Create(k KubectlRunner) error {
	_, err := k.RunWithStdin(cr.YAML, "apply", "-f", "-", "-n", cr.Namespace)
	if err != nil {
		log.Printf("failed to create %s %s: %v", cr.Kind, cr.Name, err)
	} else {
		log.Printf("created %s %s in %s", cr.Kind, cr.Name, cr.Namespace)
	}
	return err
}

func (cr CustomResource) Delete(k KubectlRunner) error {
	_, err := k.Run("delete", strings.ToLower(cr.Kind), cr.Name, "-n", cr.Namespace, "--ignore-not-found=true")
	if err != nil {
		return fmt.Errorf("failed to delete %s %s: %w", cr.Kind, cr.Name, err)
	}
	return nil
}

func DeleteNameSpace(k KubectlRunner, namespace string) {
	if _, err := k.Run("delete", "namespace", namespace, "--ignore-not-found=true", "--wait=true"); err != nil {
		log.Printf("cleanup: failed to delete namespace %q: %v", namespace, err)
	}
}

// ScenarioCleanup removes temp dirs, apps, and namespaces on both clusters. Best-effort.
func ScenarioCleanup(paths ScenarioPaths, srcApp, tgtApp K8sDeployApp, srcKubectl, tgtKubectl KubectlRunner, namespace string) {
	if err := CleanupScenario(paths.TempDir, srcApp, tgtApp); err != nil {
		log.Printf("cleanup: %v", err)
	}
	for _, k := range []KubectlRunner{srcKubectl, tgtKubectl} {
		DeleteNameSpace(k, namespace)
	}
}

// ValidateDirResources checks that a directory exists and contains files matching the given glob patterns.
func ValidateDirResources(path string, resources []string) error {
	_, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("directory not found at %s: %w", path, err)
	}

	for _, resource := range resources {
		matches, err := filepath.Glob(filepath.Join(path, resource))
		if err != nil {
			return fmt.Errorf("glob error for %s at %s: %w", resource, path, err)
		}
		if len(matches) == 0 {
			return fmt.Errorf("no files matching %s at %s", resource, path)
		}
		log.Printf("found %d file(s) matching %s at %s", len(matches), resource, path)
	}
	return nil
}

// ValidatePipelineClusterResources verifies cluster resource files exist across all pipeline stages (export, transform, apply).
// Pass nil for transformStages to default to 10_KubernetesPlugin.
func ValidatePipelineClusterResources(paths ScenarioPaths, namespace string, resources []string, transformStages *[]string) error {
	exportClusterDir := filepath.Join(paths.ExportDir, "resources", namespace, "_cluster")
	log.Printf("Validating Export _cluster directory")
	if err := ValidateDirResources(exportClusterDir, resources); err != nil {
		return fmt.Errorf("Export: %w", err)
	}

	stages := []string{"10_KubernetesPlugin"}
	if transformStages != nil && len(*transformStages) > 0 {
		stages = *transformStages
	}
	for _, stage := range stages {
		transformClusterDir := filepath.Join(paths.TransformDir, ".work", stage, "output", "_cluster")
		log.Printf("Validating Transform(%s) _cluster directory", stage)
		if err := ValidateDirResources(transformClusterDir, resources); err != nil {
			return fmt.Errorf("Transform(%s): %w", stage, err)
		}
	}

	outputClusterDir := filepath.Join(paths.OutputDir, "resources", "_cluster")
	log.Printf("Validating Apply _cluster directory")
	if err := ValidateDirResources(outputClusterDir, resources); err != nil {
		return fmt.Errorf("Apply: %w", err)
	}

	return nil
}

// RunPipeline sets the work dir and runs crane export, transform, and apply.
func RunPipeline(runner *CraneRunner, namespace string, paths ScenarioPaths) error {
	runner.WorkDir = paths.TempDir
	log.Printf("Running crane pipeline for namespace %s", namespace)
	if err := RunCranePipelineWithChecks(*runner, namespace, paths); err != nil {
		return fmt.Errorf("pipeline failed: %w", err)
	}
	log.Printf("Crane pipeline completed")
	return nil
}

// ScaleAndValidateTargetApp scales the deployment to n replica and waits up to 2m for the app to validate.
func ScaleAndValidateTargetApp(kubectlTgt KubectlRunner, tgtApp K8sDeployApp, namespace, appName string) {
	gomega.Expect(kubectlTgt.ScaleDeployment(namespace, appName, 1)).NotTo(gomega.HaveOccurred())
	gomega.Eventually(tgtApp.Validate, "2m", "10s").Should(gomega.Succeed())
	log.Printf("Target app validated successfully")
}

type ExpectedClusterRoleBinding struct {
	ClusterRoleBindingName string
	ClusterRoleName        string
	SubjectName            string
}

// ValidateClusterRBAC verifies that each CRB exists, references the expected ClusterRole, and has the expected subject.
func ValidateClusterRBAC(kubectl KubectlRunner, namespace string, bindings []ExpectedClusterRoleBinding) error {
	clusterRoles := map[string]bool{}
	for _, b := range bindings {
		clusterRoles[b.ClusterRoleName] = true
	}
	for cr := range clusterRoles {
		if _, err := kubectl.Run("get", "clusterrole", cr); err != nil {
			return fmt.Errorf("ClusterRole %s not found: %w", cr, err)
		}
		log.Printf("ClusterRole %s exists", cr)
	}

	for _, b := range bindings {
		if _, err := kubectl.Run("get", "clusterrolebinding", b.ClusterRoleBindingName); err != nil {
			return fmt.Errorf("ClusterRoleBinding %s not found: %w", b.ClusterRoleBindingName, err)
		}

		roleRef, err := kubectl.Run("get", "clusterrolebinding", b.ClusterRoleBindingName, "-o", "jsonpath={.roleRef.name}")
		if err != nil {
			return fmt.Errorf("failed to get roleRef for CRB %s: %w", b.ClusterRoleBindingName, err)
		}
		if roleRef != b.ClusterRoleName {
			return fmt.Errorf("CRB %s references %s, expected %s", b.ClusterRoleBindingName, roleRef, b.ClusterRoleName)
		}

		subject, err := kubectl.Run("get", "clusterrolebinding", b.ClusterRoleBindingName, "-o", "jsonpath={.subjects[0].name}")
		if err != nil {
			return fmt.Errorf("failed to get subject for CRB %s: %w", b.ClusterRoleBindingName, err)
		}
		if subject != b.SubjectName {
			return fmt.Errorf("CRB %s subject is %s, expected %s", b.ClusterRoleBindingName, subject, b.SubjectName)
		}
		log.Printf("CRB %s -> CR %s (subject: %s) verified", b.ClusterRoleBindingName, b.ClusterRoleName, b.SubjectName)
	}
	return nil
}

func AssertNoClusterResources(basePath string) error {
	_, err := os.Stat(basePath)
	if err != nil {
		log.Printf("directory does not exist at %s (no cluster resources possible)", basePath)
		return nil
	}

	var found []string
	filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), "Cluster") {
			found = append(found, path)
		}
		return nil
	})

	if len(found) > 0 {
		return fmt.Errorf("found %d cluster resource(s) under %s: %v", len(found), basePath, found)
	}
	log.Printf("No cluster resources found under %s", basePath)
	return nil
}

func NonAdminApplyOutput(kubectlTgt KubectlRunner, path string, namespace string) error {
	outputNamespacedir := filepath.Join(path, "resources", namespace)
	_, err := os.Stat(outputNamespacedir)
	if err != nil {
		log.Printf("failures: %v \n", err)
		return err
	}
	err = kubectlTgt.ApplyDir(outputNamespacedir)
	return err
}

func CreateNamespaceAndDryRun(kubectl KubectlRunner, namespace, outputDir string) error {
	log.Printf("Creating namespace %s on target", namespace)
	if _, err := kubectl.Run("create", "namespace", namespace); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}
	return kubectl.ValidateApplyDir(outputDir)
}
