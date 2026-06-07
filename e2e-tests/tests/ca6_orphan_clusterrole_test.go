package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster-level export filtering", func() {
	It("[CA-6] Should not export orphan ClusterRole with no CRB linking it to exported SAs", Label("cluster-admin"), func() {
		appName := "nginx-with-serviceaccount"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		saName := "nginx-sa"
		subject := "--serviceaccount=" + namespace + ":" + saName
		readClusterRole := "crane-e2e-pod-reader"
		writeClusterRole := "crane-e2e-pod-writer"
		readClusterRoleBindingName := "reader-crane-e2e-pod-binding"
		relatedResources := []string{"ClusterRole_*" + readClusterRole + "*.yaml", "ClusterRoleBinding_*" +
			readClusterRoleBindingName + "*.yaml"}
		orphanResource := []string{"ClusterRole_" + writeClusterRole}
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
		paths, err := NewScenarioPaths("crane-ca6-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: readClusterRoleBindingName},
				ClusterRole{Name: readClusterRole},
				ClusterRole{Name: writeClusterRole},
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app with ServiceAccount on source cluster")
		prepareSrcErr := PrepareSourceApp(srcApp, kubectlSrc)
		Expect(prepareSrcErr).NotTo(HaveOccurred())

		By("Creating ClusterRole with pod read permissions")
		readCR := ClusterRole{Name: readClusterRole, Permission: "read"}
		Expect(readCR.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating orphan ClusterRole with pod write permissions (no CRB)")
		writeCR := ClusterRole{Name: writeClusterRole, Permission: "write"}
		Expect(writeCR.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding linking read ClusterRole to ServiceAccount")
		readCRB := ClusterRoleBinding{Name: readClusterRoleBindingName, ClusterRoleName: readClusterRole, Subject: subject}
		Expect(readCRB.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying orphan ClusterRole is not in export _cluster directory")
		exportClusterPath := filepath.Join(paths.ExportDir, "resources", namespace, "_cluster")
		//we dont expect to find the orphan clusterRole on the export dir.
		Expect(ValidateDirResources(exportClusterPath, orphanResource)).To(HaveOccurred())

		By("Verifying linked ClusterRole and ClusterRoleBinding exist in export, transform, and output")
		Expect(ValidatePipelineClusterResources(paths, namespace, relatedResources, nil)).NotTo(HaveOccurred())
	})
})
