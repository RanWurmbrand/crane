package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	"github.com/konveyor/crane/e2e-tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster-level RBAC export", func() {
	It("[CA-9] Should export CRD to _cluster and custom resource to namespace directory", Label("cluster-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		scenario := NewMigrationScenario(
			appName,
			namespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		srcApp := scenario.SrcApp
		tgtApp := scenario.TgtApp
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		runner := scenario.Crane
		crdPatterns := []string{"CustomResourceDefinition_*.yaml"}
		crPatterns := []string{"Widget_*.yaml"}

		crName := "test-widget"

		paths, err := NewScenarioPaths("crane-ca9-*")
		Expect(err).NotTo(HaveOccurred())
		crdYAML, err := utils.ReadTestdataFile("widget_crd.yaml")
		Expect(err).NotTo(HaveOccurred())
		crYAML, err := utils.ReadTestdataFile("widget_cr.yaml")
		Expect(err).NotTo(HaveOccurred())

		crd := CustomResourceDefinition{
			Name: "widgets.crane-e2e.example.com",
			YAML: crdYAML,
		}
		cr := CustomResource{
			Name:      "test-widget",
			Namespace: namespace,
			Kind:      "Widget",
			YAML:      crYAML,
		}
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				cr, crd,
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app on source cluster")
		Expect(PrepareSourceApp(srcApp, kubectlSrc)).NotTo(HaveOccurred())

		By("Creating Widget CRD on source")
		Expect(crd.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for CRD to be established")
		Expect(crd.WaitForEstablished(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating Widget custom resource in namespace")
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying CRD exists in export _cluster directory")
		exportClusterPath := filepath.Join(paths.ExportDir, "resources", namespace, "_cluster")
		Expect(utils.AssertKindsInOutput(exportClusterPath, []string{"CustomResourceDefinition"})).NotTo(HaveOccurred())

		By("Verifying Widget CR exists in namespace export directory")
		namespaceDir := filepath.Join(paths.ExportDir, "resources", namespace)
		Expect(ValidateDirResources(namespaceDir, crPatterns)).NotTo(HaveOccurred())

		By("Verifying CRD exists in output _cluster directory")
		outputClusterPath := filepath.Join(paths.OutputDir, "resources", "_cluster")
		Expect(ValidateDirResources(outputClusterPath, crdPatterns)).NotTo(HaveOccurred())

		By("Creating namespace on target cluster")
		Expect(kubectlTgt.CreateNamespace(namespace)).NotTo(HaveOccurred())

		By("Applying cluster resources to target")
		Expect(kubectlTgt.ApplyDir(filepath.Join(paths.OutputDir, "resources", "_cluster"))).NotTo(HaveOccurred())

		By("Waiting for CRD to be established on target")
		Expect(crd.WaitForEstablished(kubectlTgt)).NotTo(HaveOccurred())

		By("Applying namespace resources to target")
		Expect(kubectlTgt.ApplyDir(filepath.Join(paths.OutputDir, "resources", namespace))).NotTo(HaveOccurred())

		By("Verifying Widget CR exists on target")
		_, err = kubectlTgt.Run("get", "widget", crName, "-n", namespace)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying Widget CR has correct spec values on target")
		color, err := kubectlTgt.Run("get", "widget", crName, "-n", namespace,
			"-o", "jsonpath={.spec.color}")
		Expect(err).NotTo(HaveOccurred())
		Expect(color).To(Equal("blue"))

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgt, tgtApp, namespace, appName)

	})

})
