package policyread

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	corev1 "k8s.io/api/core/v1"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// AIAnalysisRuntimeConfigMapName and SignalProcessingRuntimeConfigMapName
	// are the existing operator-generated runtime configuration identities.
	AIAnalysisRuntimeConfigMapName       = "aianalysis-config"
	SignalProcessingRuntimeConfigMapName = "signalprocessing-config"
	policyRuntimeConfigKey               = "config.yaml"
)

type aiAnalysisRuntimeConfig struct {
	Rego struct {
		PolicyConfigMapName string `yaml:"policyConfigMapName"`
		PolicyConfigMapKey  string `yaml:"policyConfigMapKey"`
		ConfidenceThreshold string `yaml:"confidenceThreshold"`
	} `yaml:"rego"`
}

type signalProcessingRuntimeConfig struct {
	Classifier struct {
		RegoConfigMapName string `yaml:"regoConfigMapName"`
		RegoConfigMapKey  string `yaml:"regoConfigMapKey"`
	} `yaml:"classifier"`
}

// KubernetesSource reads only the explicitly required ConfigMap keys and CRD
// list fields through a controller-runtime reader. It never returns arbitrary
// objects to the projection service.
type KubernetesSource struct {
	reader client.Reader
}

// NewKubernetesSource creates a typed source over the AF service-account client.
func NewKubernetesSource(reader client.Reader) (*KubernetesSource, error) {
	if reader == nil {
		return nil, fmt.Errorf("kubernetes policy source reader is required")
	}
	return &KubernetesSource{reader: reader}, nil
}

// Read returns one configured ConfigMap data key. The caller hashes the bytes
// immediately and never exposes them in a response.
func (s *KubernetesSource) Read(ctx context.Context, namespace, name, key string) ([]byte, error) {
	value, err := s.readConfigMapKey(ctx, namespace, name, key)
	if err != nil {
		return nil, fmt.Errorf("read policy ConfigMap %s/%s: %w", namespace, name, err)
	}
	return value, nil
}

// ReadCurrentPolicies reads the current policy identities, revisions, and
// threshold from the existing operator-generated runtime ConfigMaps. It does
// not cache either runtime configuration or policy content.
func (s *KubernetesSource) ReadCurrentPolicies(ctx context.Context, namespace string) ([]PolicyMetadata, error) {
	aiConfigData, err := s.readConfigMapKey(ctx, namespace, AIAnalysisRuntimeConfigMapName, policyRuntimeConfigKey)
	if err != nil {
		return nil, fmt.Errorf("read AIAnalysis runtime configuration: %w", err)
	}
	var aiConfig aiAnalysisRuntimeConfig
	if err := yaml.Unmarshal(aiConfigData, &aiConfig); err != nil {
		return nil, fmt.Errorf("parse AIAnalysis runtime configuration: %w", err)
	}
	threshold, err := parseRuntimeThreshold(aiConfig.Rego.ConfidenceThreshold)
	if err != nil {
		return nil, fmt.Errorf("validate AIAnalysis runtime threshold: %w", err)
	}
	aiName := strings.TrimSpace(aiConfig.Rego.PolicyConfigMapName)
	if err := validateRuntimePolicyName(aiName, "AIAnalysis policy ConfigMap"); err != nil {
		return nil, err
	}
	aiKey := strings.TrimSpace(aiConfig.Rego.PolicyConfigMapKey)
	if err := validateRuntimePolicyKey(aiKey, "AIAnalysis policy ConfigMap key"); err != nil {
		return nil, err
	}

	spConfigData, err := s.readConfigMapKey(ctx, namespace, SignalProcessingRuntimeConfigMapName, policyRuntimeConfigKey)
	if err != nil {
		return nil, fmt.Errorf("read SignalProcessing runtime configuration: %w", err)
	}
	var spConfig signalProcessingRuntimeConfig
	if err := yaml.Unmarshal(spConfigData, &spConfig); err != nil {
		return nil, fmt.Errorf("parse SignalProcessing runtime configuration: %w", err)
	}
	spName := strings.TrimSpace(spConfig.Classifier.RegoConfigMapName)
	if err := validateRuntimePolicyName(spName, "SignalProcessing policy ConfigMap"); err != nil {
		return nil, err
	}
	spKey := strings.TrimSpace(spConfig.Classifier.RegoConfigMapKey)
	if err := validateRuntimePolicyKey(spKey, "SignalProcessing policy ConfigMap key"); err != nil {
		return nil, err
	}

	aiValue, err := s.Read(ctx, namespace, aiName, aiKey)
	if err != nil {
		return nil, fmt.Errorf("read AIAnalysis policy metadata: %w", err)
	}
	spValue, err := s.Read(ctx, namespace, spName, spKey)
	if err != nil {
		return nil, fmt.Errorf("read SignalProcessing policy metadata: %w", err)
	}
	return []PolicyMetadata{
		{
			Type:          "aiAnalysis",
			ConfigMapName: aiName,
			ConfigMapKey:  aiKey,
			Revision:      revision(aiValue),
			Threshold:     &Threshold{Value: threshold, Source: "runtime-config", Kind: "rego-approval"},
		},
		{
			Type:          "signalProcessing",
			ConfigMapName: spName,
			ConfigMapKey:  spKey,
			Revision:      revision(spValue),
		},
	}, nil
}

func (s *KubernetesSource) readConfigMapKey(ctx context.Context, namespace, name, key string) ([]byte, error) {
	var configMap corev1.ConfigMap
	if err := s.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &configMap); err != nil {
		return nil, err
	}
	value, ok := configMap.Data[key]
	if !ok {
		return nil, fmt.Errorf("ConfigMap does not contain key %q", key)
	}
	return []byte(value), nil
}

func parseRuntimeThreshold(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return 0, fmt.Errorf("confidenceThreshold must be finite and in range (0.0, 1.0]")
	}
	return value, nil
}

func validateRuntimePolicyName(name, description string) error {
	if name == "" {
		return fmt.Errorf("%s is required", description)
	}
	if errs := utilvalidation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("%s must be a DNS-1123 name: %s", description, strings.Join(errs, "; "))
	}
	return nil
}

func validateRuntimePolicyKey(key, description string) error {
	if key == "" {
		return fmt.Errorf("%s is required", description)
	}
	if errs := utilvalidation.IsConfigMapKey(key); len(errs) > 0 {
		return fmt.Errorf("%s is invalid: %s", description, strings.Join(errs, "; "))
	}
	return nil
}

// ListAIAnalysis returns typed history records for exact-target matching.
func (s *KubernetesSource) ListAIAnalysis(ctx context.Context) ([]aianalysisv1.AIAnalysis, error) {
	var records []aianalysisv1.AIAnalysis
	if err := s.ForEachAIAnalysis(ctx, func(record aianalysisv1.AIAnalysis) error {
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, err
	}
	return records, nil
}

// ListSignalProcessing returns typed history records for exact-target matching.
func (s *KubernetesSource) ListSignalProcessing(ctx context.Context) ([]spv1.SignalProcessing, error) {
	var records []spv1.SignalProcessing
	if err := s.ForEachSignalProcessing(ctx, func(record spv1.SignalProcessing) error {
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, err
	}
	return records, nil
}

// ForEachAIAnalysis traverses bounded typed pages and discards each backend
// page after its records have been handed to the projection callback.
func (s *KubernetesSource) ForEachAIAnalysis(ctx context.Context, visit func(aianalysisv1.AIAnalysis) error) error {
	continuation := ""
	seenContinuations := make(map[string]struct{})
	for {
		var list aianalysisv1.AIAnalysisList
		if err := s.reader.List(ctx, &list, &client.ListOptions{Limit: defaultInventoryBackendPageLimit, Continue: continuation}); err != nil {
			return fmt.Errorf("list AIAnalysis history page: %w", err)
		}
		for index := range list.Items {
			if err := visit(list.Items[index]); err != nil {
				return err
			}
		}
		nextContinuation := list.Continue
		if err := validateHistoryContinuation(continuation, nextContinuation, seenContinuations, "AIAnalysis"); err != nil {
			return err
		}
		list.Items = nil
		if nextContinuation == "" {
			return nil
		}
		continuation = nextContinuation
	}
}

// ForEachSignalProcessing traverses bounded typed pages and discards each
// backend page after its records have been handed to the projection callback.
func (s *KubernetesSource) ForEachSignalProcessing(ctx context.Context, visit func(spv1.SignalProcessing) error) error {
	continuation := ""
	seenContinuations := make(map[string]struct{})
	for {
		var list spv1.SignalProcessingList
		if err := s.reader.List(ctx, &list, &client.ListOptions{Limit: defaultInventoryBackendPageLimit, Continue: continuation}); err != nil {
			return fmt.Errorf("list SignalProcessing history page: %w", err)
		}
		for index := range list.Items {
			if err := visit(list.Items[index]); err != nil {
				return err
			}
		}
		nextContinuation := list.Continue
		if err := validateHistoryContinuation(continuation, nextContinuation, seenContinuations, "SignalProcessing"); err != nil {
			return err
		}
		list.Items = nil
		if nextContinuation == "" {
			return nil
		}
		continuation = nextContinuation
	}
}

func validateHistoryContinuation(continuation, nextContinuation string, seenContinuations map[string]struct{}, resourceName string) error {
	if nextContinuation == "" {
		return nil
	}
	if nextContinuation == continuation {
		return fmt.Errorf("%w: %s history continuation did not progress", inventory.ErrUnavailable, resourceName)
	}
	if _, seen := seenContinuations[nextContinuation]; seen {
		return fmt.Errorf("%w: %s history continuation repeated", inventory.ErrUnavailable, resourceName)
	}
	seenContinuations[nextContinuation] = struct{}{}
	return nil
}

var _ PolicySource = (*KubernetesSource)(nil)
var _ CurrentPolicySource = (*KubernetesSource)(nil)
var _ HistorySource = (*KubernetesSource)(nil)
var _ StreamingHistorySource = (*KubernetesSource)(nil)
