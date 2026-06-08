/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ========================================
// CNV DetectedLabels Schema/Serialization Tests — #1378
// ========================================

func TestDetectedLabelsSchema_CNV_VirtualMachineTrue(t *testing.T) {
	// UT-DS-1378-020: DetectedLabelsSchema validates virtualMachine="true"
	yamlData := "virtualMachine: \"true\""
	var schema DetectedLabelsSchema
	if err := yaml.Unmarshal([]byte(yamlData), &schema); err != nil {
		// Before Phase 5 implementation, this will fail with "unknown field"
		t.Fatalf("UT-DS-1378-020: unmarshal error (CNV fields not yet implemented): %v", err)
	}
	if err := schema.ValidateDetectedLabels(); err != nil {
		t.Errorf("UT-DS-1378-020: validation error for virtualMachine=true: %v", err)
	}
}

func TestDetectedLabelsSchema_CNV_StorageBackendODFCeph(t *testing.T) {
	// UT-DS-1378-021: DetectedLabelsSchema validates storageBackend="odf-ceph"
	yamlData := "storageBackend: odf-ceph"
	var schema DetectedLabelsSchema
	if err := yaml.Unmarshal([]byte(yamlData), &schema); err != nil {
		t.Fatalf("UT-DS-1378-021: unmarshal error (CNV fields not yet implemented): %v", err)
	}
	if err := schema.ValidateDetectedLabels(); err != nil {
		t.Errorf("UT-DS-1378-021: validation error for storageBackend=odf-ceph: %v", err)
	}
}

func TestDetectedLabelsSchema_CNV_RejectsInvalidStorageBackend(t *testing.T) {
	// UT-DS-1378-022: schema rejects storageBackend="invalid"
	yamlData := "storageBackend: invalid"
	var schema DetectedLabelsSchema
	if err := yaml.Unmarshal([]byte(yamlData), &schema); err != nil {
		t.Fatalf("UT-DS-1378-022: unmarshal error (CNV fields not yet implemented): %v", err)
	}
	if err := schema.ValidateDetectedLabels(); err == nil {
		t.Error("UT-DS-1378-022: expected validation error for invalid storageBackend")
	}
}

func TestDetectedLabelsSchema_CNV_RejectsBooleanFalse(t *testing.T) {
	// UT-DS-1378-023: schema rejects virtualMachine="false" (booleans only accept "true")
	yamlData := "virtualMachine: \"false\""
	var schema DetectedLabelsSchema
	if err := yaml.Unmarshal([]byte(yamlData), &schema); err != nil {
		t.Fatalf("UT-DS-1378-023: unmarshal error (CNV fields not yet implemented): %v", err)
	}
	if err := schema.ValidateDetectedLabels(); err == nil {
		t.Error("UT-DS-1378-023: expected validation error for virtualMachine=false")
	}
}

func TestSerializeLabels_CNV_IncludesWhenSet(t *testing.T) {
	// UT-DS-1378-024: SerializeLabels includes CNV fields when set
	dl := DetectedLabels{
		VirtualMachine: true,
		LiveMigratable: true,
		CDIManaged:     true,
		StorageBackend: "odf-ceph",
	}
	data, err := dl.SerializeLabels()
	if err != nil {
		t.Fatalf("UT-DS-1378-024: unexpected error: %v", err)
	}
	str := string(data)
	for _, key := range []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"} {
		if !strings.Contains(str, key) {
			t.Errorf("UT-DS-1378-024: expected JSON to contain %q, got: %s", key, str)
		}
	}
}

func TestSerializeLabels_CNV_OmitsWhenFalseEmpty(t *testing.T) {
	// UT-DS-1378-025: SerializeLabels omits CNV fields when false/empty
	dl := DetectedLabels{
		VirtualMachine: false,
		LiveMigratable: false,
		CDIManaged:     false,
		StorageBackend: "",
	}
	data, err := dl.SerializeLabels()
	if err != nil {
		t.Fatalf("UT-DS-1378-025: unexpected error: %v", err)
	}
	str := string(data)
	for _, key := range []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"} {
		if strings.Contains(str, key) {
			t.Errorf("UT-DS-1378-025: JSON should not contain %q when zero, got: %s", key, str)
		}
	}
}

func TestIsEmpty_CNV_VirtualMachineNotEmpty(t *testing.T) {
	dl := &DetectedLabels{VirtualMachine: true}
	if dl.IsEmpty() {
		t.Error("DetectedLabels with VirtualMachine=true should not be empty")
	}
}

func TestIsEmpty_CNV_StorageBackendNotEmpty(t *testing.T) {
	dl := &DetectedLabels{StorageBackend: "odf-ceph"}
	if dl.IsEmpty() {
		t.Error("DetectedLabels with StorageBackend=odf-ceph should not be empty")
	}
}

func TestValidDetectedLabelFields_ContainsCNV(t *testing.T) {
	expected := []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"}
	fieldSet := make(map[string]bool, len(ValidDetectedLabelFields))
	for _, f := range ValidDetectedLabelFields {
		fieldSet[f] = true
	}
	for _, f := range expected {
		if !fieldSet[f] {
			t.Errorf("ValidDetectedLabelFields missing CNV field %q", f)
		}
	}
}
