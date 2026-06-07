package e2e

import (
	"log"
	"os"
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace-admin cluster-level migration", func() {
	It("[NA-6] Should produce consistent output when export has missing cluster resources", Label("namespace-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		subject := "--serviceaccount=" + namespace + ":default"
		scenario := NewMigrationScenario(
			appName,
			namespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		clusterRoleBindingName := "crane-e2e-pod-reader-binding"
		clusterRoleName := "crane-e2e-pod-reader"
		forbiddenResourcesPatterns := []string{"clusterroles.yaml", "clusterrolebindings.yaml"}
		if scenario.KubectlSrcNonAdmin.Context == "" {
			Skip("source-nonadmin-context is required for non-admin role migration test")
		}
		if scenario.KubectlTgtNonAdmin.Context == "" {
			Skip("target-nonadmin-context is required for non-admin role migration test")
		}
		srcAppNonAdmin, tgtAppNonAdmin := NonAdminApps(scenario)
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		paths, err := NewScenarioPaths("crane-na6-*")
		runner := scenario.CraneNonAdmin
		Expect(err).NotTo(HaveOccurred())

		By("Granting namespace-admin permissions to non-admin user on source and target")
		kubectlSrcNonAdmin, kubectlTgtNonAdmin, rbacCleanup, err := SetupNamespaceAdminUsersForScenario(scenario, namespace)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(rbacCleanup)
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: clusterRoleBindingName},
				ClusterRole{Name: clusterRoleName},
			})
		})
		DeferCleanup(func() {
			ScenarioCleanup(paths, srcAppNonAdmin, tgtAppNonAdmin, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app as namespace-admin on source cluster")
		err = PrepareSourceApp(srcAppNonAdmin, kubectlSrcNonAdmin)
		Expect(err).NotTo(HaveOccurred())

		By("Creating ClusterRole as cluster-admin")
		cr := ClusterRole{Name: clusterRoleName, Permission: "read"}
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding as cluster-admin")
		crb := ClusterRoleBinding{Name: clusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: subject}
		Expect(crb.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrcNonAdmin, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply as namespace-admin")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying cluster resource failures were recorded in failures directory")
		failuresDir := filepath.Join(paths.ExportDir, "failures", namespace)
		Expect(ValidateDirResources(failuresDir, forbiddenResourcesPatterns)).NotTo(HaveOccurred())

		By("Verifying output _cluster files match transform _cluster files")
		outputClusterPath := filepath.Join(paths.OutputDir, "resources", "_cluster")
		if _, err := os.Stat(outputClusterPath); os.IsNotExist(err) {
			log.Printf("No _cluster directory in output (expected for thin export)")
		} else {
			files, err := filepath.Glob(filepath.Join(outputClusterPath, "*.yaml"))
			Expect(err).NotTo(HaveOccurred())
			log.Printf("output _cluster contains %d files", len(files))

			transformClusterPath := filepath.Join(paths.TransformDir, ".work", "10_KubernetesPlugin", "output", "_cluster")
			for _, outputFile := range files {
				baseName := filepath.Base(outputFile)
				transformFile := filepath.Join(transformClusterPath, baseName)
				Expect(transformFile).To(BeAnExistingFile(),
					"output file %s has no corresponding transform file", baseName)
			}
		}

		By("Verifying namespace resources exist in output directory")
		namespaceDir := filepath.Join(paths.OutputDir, "resources", namespace)
		Expect(namespaceDir).To(BeADirectory())
		deploymentPattern := filepath.Join(namespaceDir, "Deployment_*.yaml")
		matches, err := filepath.Glob(deploymentPattern)
		Expect(err).NotTo(HaveOccurred())
		Expect(matches).NotTo(BeEmpty(), "expected Deployment in namespace output")

		By("Creating namespace on target cluster")
		Expect(kubectlTgt.CreateNamespace(namespace)).NotTo(HaveOccurred())

		By("Applying namespace resources to target as namespace-admin")
		Expect(kubectlTgtNonAdmin.ApplyDir(filepath.Join(paths.OutputDir, "resources", namespace))).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgtNonAdmin, tgtAppNonAdmin, namespace, appName)

	})

})
