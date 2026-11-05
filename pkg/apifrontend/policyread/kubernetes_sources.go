package policyread

import (
	"context"
	"fmt"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
	var configMap corev1.ConfigMap
	if err := s.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &configMap); err != nil {
		return nil, fmt.Errorf("get policy ConfigMap %s/%s: %w", namespace, name, err)
	}
	value, ok := configMap.Data[key]
	if !ok {
		return nil, fmt.Errorf("policy ConfigMap %s/%s does not contain key %q", namespace, name, key)
	}
	return []byte(value), nil
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
var _ HistorySource = (*KubernetesSource)(nil)
var _ StreamingHistorySource = (*KubernetesSource)(nil)
