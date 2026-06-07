package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster-level export filtering", func() {
	It("[CA-8] Should export only labeled workload and its RBAC with --label-selector", Label("cluster-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		inScopeSaName := "nginx-sa"
		inScopeCr := "crane-e2e-pod-reader"
		inScopeCRB := "reader-crane-e2e-pod-binding"
		inScopesubject := "--serviceaccount=" + namespace + ":" + inScopeSaName
		inScopeResources := []string{"ClusterRole_*" + inScopeCr + "*.yaml", "ClusterRoleBinding_*" +
			inScopeCRB + "*.yaml"}
		outOfScopeSaName := "out-of-scope-sa"
		outScoptCr := "out-of-scope-pod-writer"
		outOfScopeCRB := "out-of-scope-pod-binding"
		outScopeubject := "--serviceaccount=" + namespace + ":" + outOfScopeSaName
		outOfScopeResource := []string{"ClusterRoleBinding_" + outOfScopeCRB + "*.yaml",
			"ClusterRole_*" + outOfScopeCRB + "*.yaml"}

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
		runner.LabelSelector = "app=" + appName
		paths, err := NewScenarioPaths("crane-ca8-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: inScopeCRB},
				ClusterRoleBinding{Name: outOfScopeCRB},
				ClusterRole{Name: inScopeCr},
				ClusterRole{Name: outScoptCr},
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app on source cluster")
		Expect(PrepareSourceApp(srcApp, kubectlSrc)).NotTo(HaveOccurred())

		By("Creating in-scope ServiceAccount with matching label")
		inScopeSA := ServiceAccount{Name: inScopeSaName, Namespace: namespace, Label: "app=" + appName}
		Expect(inScopeSA.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating out-of-scope ServiceAccount with different label")
		outOfScopeSA := ServiceAccount{Name: outOfScopeSaName, Namespace: namespace, Label: "app=outScopedApp"}
		Expect(outOfScopeSA.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating in-scope ClusterRole with matching label")
		inScopeCR := ClusterRole{Name: inScopeCr, Permission: "read", Label: "app=" + appName}
		Expect(inScopeCR.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating out-of-scope ClusterRole with different label")
		outOfScopeCR := ClusterRole{Name: outScoptCr, Permission: "write", Label: "app=outScopedApp"}
		Expect(outOfScopeCR.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating in-scope ClusterRoleBinding with matching label")
		inScopeBinding := ClusterRoleBinding{Name: inScopeCRB, ClusterRoleName: inScopeCr, Subject: inScopesubject, Label: "app=" + appName}
		Expect(inScopeBinding.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating out-of-scope ClusterRoleBinding with different label")
		outOfScopeBinding := ClusterRoleBinding{Name: outOfScopeCRB, ClusterRoleName: outScoptCr, Subject: outScopeubject, Label: "app=outScopedApp"}
		Expect(outOfScopeBinding.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export with label-selector, transform, apply")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying out-of-scope resources are not in export _cluster directory")
		exportClusterPath := filepath.Join(paths.ExportDir, "resources", namespace, "_cluster")
		Expect(ValidateDirResources(exportClusterPath, outOfScopeResource)).To(HaveOccurred())

		By("Verifying in-scope ClusterRole and ClusterRoleBinding exist in export, transform, and output")
		Expect(ValidatePipelineClusterResources(paths, namespace, inScopeResources, nil)).NotTo(HaveOccurred())
	})
})
