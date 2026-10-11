package apifrontend_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BR-PLATFORM-001.6a: least-privileged external collector preparation (AC-6, SC-8)", func() {
	It("IT-AF-2501-003: actual lane preparation grants only existing DS get permission and cleans owned credentials", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		dir := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		Expect(os.WriteFile(filepath.Join(dir, "podman"), []byte("#!/bin/bash\nprintf '10.89.0.5'\n"), 0755)).To(Succeed())
		ca := infrastructure.BootstrapTLSCAFile("https://localhost:18096")
		service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "data-storage-service", Namespace: defaultFixture}, Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort, Ports: []corev1.ServicePort{{Port: 8080, NodePort: 30089}},
		}}
		Expect(k8sClient.Create(ctx, service)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), service)).To(Succeed()) })
		opts := infrastructure.RunMustGatherImageOptions{ClusterName: "owned-query-2501", Namespace: defaultFixture, OutputDir: filepath.Join(dir, "output")}
		kubeconfig, err := infrastructure.WriteEnvtestKubeconfigToFile(restCfg, "collector-"+uuid.NewString())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(os.Remove(kubeconfig)).To(Succeed()) })
		cleanup, err := infrastructure.PrepareMustGatherDataStorage(ctx, &opts, kubeconfig, ca)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(cleanup()).To(Succeed()) })
		Expect(opts.DataStorage).ToNot(BeNil())
		Expect(opts.DataStorage.URL).To(Equal("https://data-storage-service.default.svc:8080"))
		Expect(opts.DataStorage.ConnectTo).To(Equal("data-storage-service.default.svc:8080:10.89.0.5:30089"))
		token, err := os.ReadFile(opts.DataStorage.TokenFile)
		Expect(err).ToNot(HaveOccurred())
		review, err := k8sClientset.AuthenticationV1().TokenReviews().Create(ctx, &authnv1.TokenReview{Spec: authnv1.TokenReviewSpec{Token: string(token)}}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(review.Status.Authenticated).To(BeTrue())
		for _, verb := range []string{"get", "create", "update", "delete"} {
			review, err := k8sClientset.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authzv1.SubjectAccessReview{Spec: authzv1.SubjectAccessReviewSpec{
				User: review.Status.User.Username, Groups: review.Status.User.Groups,
				ResourceAttributes: &authzv1.ResourceAttributes{Namespace: defaultFixture, Verb: verb, Resource: "services", Name: "data-storage-service"},
			}}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(review.Status.Allowed).To(Equal(verb == "get"), "collector must not inherit write authority")
		}
		Expect(cleanup()).To(Succeed())
		_, err = os.Stat(opts.DataStorage.TokenFile)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
