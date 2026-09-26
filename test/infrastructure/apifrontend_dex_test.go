package infrastructure

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

const dexDataVolumeName = "dex-data"

type dexE2EManifestDocument struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Data map[string]string `yaml:"data"`
	Spec struct {
		Replicas    int      `yaml:"replicas"`
		AccessModes []string `yaml:"accessModes"`
		Resources   struct {
			Requests map[string]string `yaml:"requests"`
		} `yaml:"resources"`
		Template struct {
			Spec struct {
				SecurityContext struct {
					FSGroup int64 `yaml:"fsGroup"`
				} `yaml:"securityContext"`
				Containers []struct {
					Name           string `yaml:"name"`
					ReadinessProbe struct {
						TimeoutSeconds int `yaml:"timeoutSeconds"`
					} `yaml:"readinessProbe"`
					Resources struct {
						Requests map[string]string `yaml:"requests"`
						Limits   map[string]string `yaml:"limits"`
					} `yaml:"resources"`
					VolumeMounts []struct {
						Name      string `yaml:"name"`
						MountPath string `yaml:"mountPath"`
					} `yaml:"volumeMounts"`
				} `yaml:"containers"`
				Volumes []struct {
					Name                  string `yaml:"name"`
					PersistentVolumeClaim struct {
						ClaimName string `yaml:"claimName"`
					} `yaml:"persistentVolumeClaim"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

var _ = Describe("API Frontend E2E DEX fixture", func() {
	It("UT-INFRA-AF-DEX-001 [Issue #1807]: persists DEX state and gives auth requests adequate CPU and probe budget", func() {
		manifestPath := filepath.Join(getProjectRoot(), "deploy/apifrontend/overlays/e2e/dex.yaml")
		manifest, err := os.ReadFile(manifestPath)
		Expect(err).NotTo(HaveOccurred())

		decoder := yaml.NewDecoder(bytes.NewReader(manifest))
		documents := make(map[string]dexE2EManifestDocument, 4)
		for {
			var document dexE2EManifestDocument
			err := decoder.Decode(&document)
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			if document.Kind != "" {
				documents[document.Kind+"/"+document.Metadata.Name] = document
			}
		}

		configMap := documents["ConfigMap/dex-config"]
		Expect(configMap.Data).To(HaveKey("config.yaml"))
		var dexConfig struct {
			Storage struct {
				Type   string `yaml:"type"`
				Config struct {
					File string `yaml:"file"`
				} `yaml:"config"`
			} `yaml:"storage"`
		}
		Expect(yaml.Unmarshal([]byte(configMap.Data["config.yaml"]), &dexConfig)).To(Succeed())
		Expect(dexConfig.Storage.Type).To(Equal("sqlite3"))
		Expect(dexConfig.Storage.Config.File).To(Equal("/var/lib/dex/dex.db"))

		deployment := documents["Deployment/dex"]
		Expect(deployment.Spec.Replicas).To(Equal(1))
		Expect(deployment.Spec.Template.Spec.SecurityContext.FSGroup).To(Equal(int64(1001)))
		Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
		dex := deployment.Spec.Template.Spec.Containers[0]
		Expect(dex.Name).To(Equal("dex"))
		Expect(dex.ReadinessProbe.TimeoutSeconds).To(Equal(3))
		Expect(dex.Resources.Requests).To(HaveKeyWithValue("cpu", "250m"))
		Expect(dex.Resources.Limits).To(HaveKeyWithValue("cpu", "500m"))
		Expect(dex.Resources.Requests).To(HaveKeyWithValue("memory", "128Mi"))
		Expect(dex.Resources.Limits).To(HaveKeyWithValue("memory", "256Mi"))

		dataVolumeMounted := false
		for _, mount := range dex.VolumeMounts {
			if mount.Name == dexDataVolumeName && mount.MountPath == "/var/lib/dex" {
				dataVolumeMounted = true
				break
			}
		}
		Expect(dataVolumeMounted).To(BeTrue())

		dataVolumeUsesPVC := false
		for _, volume := range deployment.Spec.Template.Spec.Volumes {
			if volume.Name == dexDataVolumeName && volume.PersistentVolumeClaim.ClaimName == dexDataVolumeName {
				dataVolumeUsesPVC = true
				break
			}
		}
		Expect(dataVolumeUsesPVC).To(BeTrue())

		pvc := documents["PersistentVolumeClaim/"+dexDataVolumeName]
		Expect(pvc.Spec.AccessModes).To(ConsistOf("ReadWriteOnce"))
		Expect(pvc.Spec.Resources.Requests).To(HaveKeyWithValue("storage", "1Gi"))
	})
})
