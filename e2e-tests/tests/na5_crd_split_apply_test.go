package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	"github.com/konveyor/crane/e2e-tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace-admin cluster-level migration", func() {
	It("[NA-5] Should migrate CRD + CR with split apply: cluster-admin applies CRD, namespace-admin applies CR", Label("namespace-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		// subject := "--serviceaccount=" + namespace + ":default"
		scenario := NewMigrationScenario(
			appName,
			namespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		crdPatterns := []string{"CustomResourceDefinition_*.yaml"}
		crPatterns := []string{"Widget_*.yaml"}
		if scenario.KubectlSrcNonAdmin.Context == "" {
			Skip("source-nonadmin-context is required for non-admin role migration test")
		}
		if scenario.KubectlTgtNonAdmin.Context == "" {
			Skip("target-nonadmin-context is required for non-admin role migration test")
		}
		srcAppNonAdmin, tgtAppNonAdmin := NonAdminApps(scenario)
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
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
		paths, err := NewScenarioPaths("crane-na5-*")

		runner := scenario.Crane
		Expect(err).NotTo(HaveOccurred())

		By("Granting namespace-admin permissions to non-admin user on source and target")
		kubectlSrcNonAdmin, kubectlTgtNonAdmin, rbacCleanup, err := SetupNamespaceAdminUsersForScenario(scenario, namespace)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(rbacCleanup)
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				cr, crd,
			})
		})
		DeferCleanup(func() {
			ScenarioCleanup(paths, srcAppNonAdmin, tgtAppNonAdmin, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app as namespace-admin on source cluster")
		err = PrepareSourceApp(srcAppNonAdmin, kubectlSrcNonAdmin)
		Expect(err).NotTo(HaveOccurred())

		By("Creating Widget CRD as cluster-admin")
		Expect(crd.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for CRD to be established")
		Expect(crd.WaitForEstablished(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating Widget custom resource as cluster-admin")
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply as cluster-admin")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying CRD exists in export _cluster directory")
		exportClusterPath := filepath.Join(paths.ExportDir, "resources", namespace, "_cluster")
		Expect(ValidateDirResources(exportClusterPath, crdPatterns)).NotTo(HaveOccurred())

		By("Verifying Widget CR exists in namespace export directory")
		namespaceDir := filepath.Join(paths.ExportDir, "resources", namespace)
		Expect(ValidateDirResources(namespaceDir, crPatterns)).NotTo(HaveOccurred())

		By("Verifying CRD exists in output _cluster directory")
		outputClusterPath := filepath.Join(paths.OutputDir, "resources", "_cluster")
		Expect(ValidateDirResources(outputClusterPath, crdPatterns)).NotTo(HaveOccurred())

		By("Creating namespace on target cluster")
		Expect(kubectlTgt.CreateNamespace(namespace)).NotTo(HaveOccurred())

		By("Applying CRD to target as cluster-admin")
		Expect(kubectlTgt.ApplyDir(filepath.Join(paths.OutputDir, "resources", "_cluster"))).NotTo(HaveOccurred())

		By("Waiting for CRD to be established on target")
		Expect(crd.WaitForEstablished(kubectlTgt)).NotTo(HaveOccurred())

		By("Granting namespace-admin permission to manage widgets on target")
		_, err = kubectlTgt.Run("create", "role", "widget-admin", "-n", namespace,
			"--verb=*", "--resource=widgets.crane-e2e.example.com")
		Expect(err).NotTo(HaveOccurred())
		_, err = kubectlTgt.Run("create", "rolebinding", "widget-admin-binding", "-n", namespace,
			"--role=widget-admin", "--user=dev")
		Expect(err).NotTo(HaveOccurred())

		By("Applying namespace resources to target as namespace-admin")
		Expect(kubectlTgtNonAdmin.ApplyDir(filepath.Join(paths.OutputDir, "resources", namespace))).NotTo(HaveOccurred())

		By("Verifying Widget CR exists on target")
		_, err = kubectlTgtNonAdmin.Run("get", "widget", "test-widget", "-n", namespace)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying Widget CR has correct spec values on target")
		color, err := kubectlTgtNonAdmin.Run("get", "widget", "test-widget", "-n", namespace,
			"-o", "jsonpath={.spec.color}")
		Expect(err).NotTo(HaveOccurred())
		Expect(color).To(Equal("blue"))

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgtNonAdmin, tgtAppNonAdmin, namespace, appName)

	})

})
