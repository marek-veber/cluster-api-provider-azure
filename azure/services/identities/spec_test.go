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

package identities

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseUserAssignedIdentityResourceID(t *testing.T) {
	tests := []struct {
		name                string
		resourceID          string
		expectedName        string
		expectedRG          string
		expectError         bool
		expectedErrorString string
	}{
		{
			name:         "valid user-assigned identity resource ID",
			resourceID:   "/subscriptions/64f0619f-ebc2-4156-9d91-c4c781de7e54/resourcegroups/mveber-int-resgroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/mveber-mveber-int-cp-file-csi-driver-46972e",
			expectedName: "mveber-mveber-int-cp-file-csi-driver-46972e",
			expectedRG:   "mveber-int-resgroup",
			expectError:  false,
		},
		{
			name:         "valid with different casing in resourceGroups",
			resourceID:   "/subscriptions/64f0619f-ebc2-4156-9d91-c4c781de7e54/resourceGroups/MyResourceGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity",
			expectedName: "my-identity",
			expectedRG:   "MyResourceGroup",
			expectError:  false,
		},
		{
			name:                "invalid resource ID format",
			resourceID:          "invalid-resource-id",
			expectError:         true,
			expectedErrorString: "failed to parse resource ID",
		},
		{
			name:                "wrong provider",
			resourceID:          "/subscriptions/64f0619f-ebc2-4156-9d91-c4c781de7e54/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm",
			expectError:         true,
			expectedErrorString: "expected resource type Microsoft.ManagedIdentity/userAssignedIdentities, got Microsoft.Compute/virtualMachines",
		},
		{
			name:                "wrong resource type",
			resourceID:          "/subscriptions/64f0619f-ebc2-4156-9d91-c4c781de7e54/resourceGroups/rg/providers/Microsoft.ManagedIdentity/systemAssignedIdentities/identity",
			expectError:         true,
			expectedErrorString: "expected resource type Microsoft.ManagedIdentity/userAssignedIdentities, got Microsoft.ManagedIdentity/systemAssignedIdentities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := ParseUserAssignedIdentityResourceID(tt.resourceID)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				if tt.expectedErrorString != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.expectedErrorString))
				}
				g.Expect(result).To(BeNil())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Name).To(Equal(tt.expectedName))
				g.Expect(result.ResourceGroup).To(Equal(tt.expectedRG))
			}
		})
	}
}

func TestGetResourceID(t *testing.T) {
	tests := []struct {
		name           string
		spec           *UserAssignedIdentitySpec
		subscriptionID string
		expectedID     string
	}{
		{
			name: "basic resource ID construction",
			spec: &UserAssignedIdentitySpec{
				Name:          "my-identity",
				ResourceGroup: "my-rg",
			},
			subscriptionID: "12345678-1234-1234-1234-123456789012",
			expectedID:     "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/my-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := tt.spec.GetResourceID(tt.subscriptionID)
			g.Expect(result).To(Equal(tt.expectedID))
		})
	}
}
