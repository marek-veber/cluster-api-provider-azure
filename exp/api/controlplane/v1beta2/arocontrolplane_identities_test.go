/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateManagedIdentities(t *testing.T) {
	tests := []struct {
		name                string
		managedIdentities   *ManagedIdentities
		expectedErrors      int
		expectedErrorFields []string
	}{
		{
			name:              "nil managed identities should pass",
			managedIdentities: nil,
			expectedErrors:    0,
		},
		{
			name:              "empty managed identities should pass",
			managedIdentities: &ManagedIdentities{},
			expectedErrors:    0,
		},
		{
			name: "valid service managed identity should pass",
			managedIdentities: &ManagedIdentities{
				ServiceManagedIdentity: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-identity",
			},
			expectedErrors: 0,
		},
		{
			name: "invalid service managed identity should fail",
			managedIdentities: &ManagedIdentities{
				ServiceManagedIdentity: "invalid-resource-id",
			},
			expectedErrors:      1,
			expectedErrorFields: []string{"spec.platform.managedIdentities.serviceManagedIdentity"},
		},
		{
			name: "valid control plane operators should pass",
			managedIdentities: &ManagedIdentities{
				ControlPlaneOperators: &ControlPlaneOperators{
					ControlPlaneManagedIdentities:           "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/control-plane",
					ClusterAPIAzureManagedIdentities:        "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/cluster-api",
					CloudControllerManagerManagedIdentities: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/ccm",
				},
			},
			expectedErrors: 0,
		},
		{
			name: "invalid control plane operators should fail",
			managedIdentities: &ManagedIdentities{
				ControlPlaneOperators: &ControlPlaneOperators{
					ControlPlaneManagedIdentities:    "invalid-id-1",
					ClusterAPIAzureManagedIdentities: "invalid-id-2",
				},
			},
			expectedErrors: 2,
			expectedErrorFields: []string{
				"spec.platform.managedIdentities.controlPlaneOperators.controlPlaneOperatorsManagedIdentities",
				"spec.platform.managedIdentities.controlPlaneOperators.clusterApiAzureManagedIdentities",
			},
		},
		{
			name: "valid data plane operators should pass",
			managedIdentities: &ManagedIdentities{
				DataPlaneOperators: &DataPlaneOperators{
					DiskCsiDriverManagedIdentities: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/disk-csi",
					FileCsiDriverManagedIdentities: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/file-csi",
				},
			},
			expectedErrors: 0,
		},
		{
			name: "mixed valid and invalid identities",
			managedIdentities: &ManagedIdentities{
				ServiceManagedIdentity: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/service",
				ControlPlaneOperators: &ControlPlaneOperators{
					ControlPlaneManagedIdentities: "invalid-control-plane-id",
				},
				DataPlaneOperators: &DataPlaneOperators{
					DiskCsiDriverManagedIdentities: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/disk-csi",
				},
			},
			expectedErrors:      1,
			expectedErrorFields: []string{"spec.platform.managedIdentities.controlPlaneOperators.controlPlaneOperatorsManagedIdentities"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create a test AROControlPlane
			aroControlPlane := &AROControlPlane{
				Spec: AROControlPlaneSpec{
					Platform: AROPlatformProfileControlPlane{
						ManagedIdentities: ManagedIdentities{},
					},
				},
			}

			if tt.managedIdentities != nil {
				aroControlPlane.Spec.Platform.ManagedIdentities = *tt.managedIdentities
			}

			// Call the validation function
			errs := aroControlPlane.validateManagedIdentities(nil)

			// Check the number of errors
			g.Expect(errs).To(HaveLen(tt.expectedErrors), "Expected %d errors, got %d: %v", tt.expectedErrors, len(errs), errs)

			// Check specific error fields if expected
			if len(tt.expectedErrorFields) > 0 {
				actualFields := make([]string, len(errs))
				for i, err := range errs {
					actualFields[i] = err.Field
				}
				for _, expectedField := range tt.expectedErrorFields {
					g.Expect(actualFields).To(ContainElement(expectedField), "Expected error field %s not found in %v", expectedField, actualFields)
				}
			}
		})
	}
}

func TestValidateUserAssignedIdentity(t *testing.T) {
	tests := []struct {
		name               string
		identityResourceID string
		expectedError      bool
		expectedMessage    string
	}{
		{
			name:               "empty identity should pass",
			identityResourceID: "",
			expectedError:      false,
		},
		{
			name:               "valid identity resource ID should pass",
			identityResourceID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-identity",
			expectedError:      false,
		},
		{
			name:               "malformed resource ID should fail",
			identityResourceID: "not-a-resource-id",
			expectedError:      true,
			expectedMessage:    "must be a valid Azure resource ID",
		},
		{
			name:               "invalid resource ID format should fail",
			identityResourceID: "/invalid/resource/id/format",
			expectedError:      true,
			expectedMessage:    "must be a valid Azure resource ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Call the validation function
			errs := validateUserAssignedIdentity(tt.identityResourceID, field.NewPath("test"))

			if tt.expectedError {
				g.Expect(errs).ToNot(BeEmpty(), "Expected validation error but got none")
				g.Expect(errs[0].Detail).To(ContainSubstring(tt.expectedMessage), "Expected error message to contain '%s', got '%s'", tt.expectedMessage, errs[0].Detail)
			} else {
				g.Expect(errs).To(BeEmpty(), "Expected no validation errors, got: %v", errs)
			}
		})
	}
}
