package remediationorchestrator

import (
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	testhelpers "github.com/jordigilh/kubernaut/test/shared/helpers"
)

const (
	trustedWorkflowName         = "generic-restart-v1"
	trustedWorkflowActionType   = "RestartPod"
	trustedWorkflowVersion      = "1.0.0"
	trustedWorkflowBundle       = "quay.io/kubernaut-cicd/tekton-bundles/hello-world:v1.0.0@sha256:a663ba9ddf8a074025723a4fbbef5542f520deb4e5eaf9814e07775456ecd7e0"
	trustedWorkflowBundleDigest = "sha256:a663ba9ddf8a074025723a4fbbef5542f520deb4e5eaf9814e07775456ecd7e0"
	trustedWorkflowEngine       = "tekton"
)

func typedStringParameters(values map[string]string) sharedtypes.TypedParameterMap {
	result := make(sharedtypes.TypedParameterMap, len(values))
	for key, value := range values {
		v := value
		result[key] = sharedtypes.TypedParameterValue{Kind: "string", StringValue: &v}
	}
	return result
}

func trustedWorkflowSnapshot() sharedtypes.WorkflowSnapshot {
	return sharedtypes.WorkflowSnapshot{
		WorkflowID:            trustedWorkflowID,
		WorkflowName:          trustedWorkflowName,
		ActionType:            trustedWorkflowActionType,
		Version:               trustedWorkflowVersion,
		ExecutionBundle:       trustedWorkflowBundle,
		ExecutionBundleDigest: trustedWorkflowBundleDigest,
		ExecutionEngine:       trustedWorkflowEngine,
	}
}

func trustedWorkflowParameters(targetKind, targetName, targetNamespace string) sharedtypes.TypedParameterMap {
	return typedStringParameters(map[string]string{
		"TARGET_RESOURCE_NAME":      targetName,
		"TARGET_RESOURCE_KIND":      targetKind,
		"TARGET_RESOURCE_NAMESPACE": targetNamespace,
		"NAMESPACE":                 targetNamespace,
		"POD_NAME":                  targetName,
	})
}

func trustedWorkflowCompletionOptions(targetKind, targetName, targetNamespace string) testhelpers.AICompletionOpts {
	return testhelpers.AICompletionOpts{
		TargetKind:         targetKind,
		TargetName:         targetName,
		TargetNamespace:    targetNamespace,
		WorkflowID:         trustedWorkflowID,
		WorkflowName:       trustedWorkflowName,
		WorkflowActionType: trustedWorkflowActionType,
		WorkflowVersion:    trustedWorkflowVersion,
		ExecutionBundle:    trustedWorkflowBundle,
		ExecutionEngine:    trustedWorkflowEngine,
		Parameters:         trustedWorkflowParameters(targetKind, targetName, targetNamespace),
	}
}
