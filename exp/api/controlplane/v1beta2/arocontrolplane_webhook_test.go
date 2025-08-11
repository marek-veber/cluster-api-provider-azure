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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

func TestAROControlPlaneWebhook_Default(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)

	testCases := []struct {
		name            string
		inputVersion    string
		expectedVersion string
		description     string
	}{
		{
			name:            "openshift-v prefix removed",
			inputVersion:    "openshift-v4.14",
			expectedVersion: "4.14",
			description:     "should remove openshift-v prefix",
		},
		{
			name:            "v prefix removed",
			inputVersion:    "v4.14",
			expectedVersion: "4.14",
			description:     "should remove v prefix",
		},
		{
			name:            "plain X.Y version unchanged",
			inputVersion:    "4.14",
			expectedVersion: "4.14",
			description:     "should leave plain X.Y version unchanged",
		},
		{
			name:            "empty version unchanged",
			inputVersion:    "",
			expectedVersion: "",
			description:     "should leave empty version unchanged",
		},
		{
			name:            "semantic version with patch stripped",
			inputVersion:    "4.14.5",
			expectedVersion: "4.14.5",
			description:     "should leave semantic version as-is for defaulter",
		},
		{
			name:            "openshift-v with patch version",
			inputVersion:    "openshift-v4.14.5",
			expectedVersion: "4.14.5",
			description:     "should handle openshift-v prefix with patch version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			controlPlane := &AROControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cp",
					Namespace: "default",
				},
				Spec: AROControlPlaneSpec{
					Version: tc.inputVersion,
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			webhook := &aroControlPlaneWebhook{Client: fakeClient}

			err := webhook.Default(context.TODO(), controlPlane)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(controlPlane.Spec.Version).To(Equal(tc.expectedVersion), tc.description)
		})
	}
}

func TestValidateOCPVersion(t *testing.T) {
	testCases := []struct {
		name        string
		version     string
		expectError bool
		description string
	}{
		{
			name:        "valid X.Y version",
			version:     "4.14",
			expectError: false,
			description: "should accept valid X.Y version format",
		},
		{
			name:        "valid X.Y version with higher numbers",
			version:     "4.20",
			expectError: false,
			description: "should accept valid X.Y version format with higher numbers",
		},
		{
			name:        "invalid semantic version with patch",
			version:     "4.14.5",
			expectError: true,
			description: "should reject full semantic version with patch",
		},
		{
			name:        "invalid version with pre-release",
			version:     "4.14.5-rc.1",
			expectError: true,
			description: "should reject version with pre-release",
		},
		{
			name:        "invalid version with build metadata",
			version:     "4.14.5+build.1",
			expectError: true,
			description: "should reject version with build metadata",
		},
		{
			name:        "invalid version with openshift-v prefix",
			version:     "openshift-v4.14",
			expectError: true,
			description: "should reject version with openshift-v prefix",
		},
		{
			name:        "invalid version with v prefix",
			version:     "v4.14",
			expectError: true,
			description: "should reject version with v prefix",
		},
		{
			name:        "invalid version format with single number",
			version:     "4",
			expectError: true,
			description: "should reject incomplete version with single number",
		},
		{
			name:        "invalid version format with letters",
			version:     "4.abc",
			expectError: true,
			description: "should reject version with letters in minor version",
		},
		{
			name:        "empty version",
			version:     "",
			expectError: true,
			description: "should reject empty version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			controlPlane := &AROControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cp",
					Namespace: "default",
				},
				Spec: AROControlPlaneSpec{
					AroClusterName: "test-cluster",
					Version:        tc.version,
					Platform: AROPlatformProfileControlPlane{
						ResourceGroup: "test-rg",
						Location:      "eastus",
					},
					Network: &NetworkSpec{
						NetworkType: "OVNKubernetes",
						MachineCIDR: "10.0.0.0/16",
						ServiceCIDR: "172.30.0.0/16",
						PodCIDR:     "10.128.0.0/14",
						HostPrefix:  23,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
			err := controlPlane.Validate(fakeClient)

			if tc.expectError {
				g.Expect(err).To(HaveOccurred(), tc.description)
			} else {
				g.Expect(err).NotTo(HaveOccurred(), tc.description)
			}
		})
	}
}

func TestSetDefaultOCPVersion(t *testing.T) {
	testCases := []struct {
		name            string
		inputVersion    string
		expectedVersion string
		description     string
	}{
		{
			name:            "openshift-v prefix removed",
			inputVersion:    "openshift-v4.14",
			expectedVersion: "4.14",
			description:     "should remove openshift-v prefix",
		},
		{
			name:            "v prefix removed",
			inputVersion:    "v4.14",
			expectedVersion: "4.14",
			description:     "should remove v prefix",
		},
		{
			name:            "plain version unchanged",
			inputVersion:    "4.14",
			expectedVersion: "4.14",
			description:     "should leave plain version unchanged",
		},
		{
			name:            "empty version unchanged",
			inputVersion:    "",
			expectedVersion: "",
			description:     "should leave empty version unchanged",
		},
		{
			name:            "semantic version with patch",
			inputVersion:    "4.14.5",
			expectedVersion: "4.14.5",
			description:     "should leave semantic version unchanged",
		},
		{
			name:            "openshift-v with patch version",
			inputVersion:    "openshift-v4.14.5-rc.1",
			expectedVersion: "4.14.5-rc.1",
			description:     "should handle openshift-v prefix with pre-release",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			result := setDefaultOCPVersion(tc.inputVersion)
			g.Expect(result).To(Equal(tc.expectedVersion), tc.description)
		})
	}
}

func TestAROControlPlaneWebhook_ValidateUpdate_ImmutableFields(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)

	baseControlPlane := &AROControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cp",
			Namespace: "default",
		},
		Spec: AROControlPlaneSpec{
			AroClusterName: "test-cluster",
			Version:        "4.14",
			ChannelGroup:   "stable",
			Platform: AROPlatformProfileControlPlane{
				ResourceGroup:          "test-rg",
				Location:               "eastus",
				NetworkSecurityGroupID: "test-nsg",
				Subnet:                 "test-subnet",
				OutboundType:           "Loadbalancer",
			},
			Visibility: "Public",
			Network: &NetworkSpec{
				NetworkType: "OVNKubernetes",
				MachineCIDR: "10.0.0.0/16",
				ServiceCIDR: "172.30.0.0/16",
				PodCIDR:     "10.128.0.0/14",
				HostPrefix:  23,
			},
		},
	}

	testCases := []struct {
		name        string
		modify      func(*AROControlPlane)
		expectError bool
		errorField  string
	}{
		{
			name: "version change should fail",
			modify: func(cp *AROControlPlane) {
				cp.Spec.Version = "4.15"
			},
			expectError: true,
			errorField:  "spec.version",
		},
		{
			name: "channelGroup change should pass",
			modify: func(cp *AROControlPlane) {
				cp.Spec.ChannelGroup = "fast"
			},
			expectError: false,
		},
		{
			name: "aroClusterName change should fail",
			modify: func(cp *AROControlPlane) {
				cp.Spec.AroClusterName = "different-cluster"
			},
			expectError: true,
			errorField:  "spec.aroClusterName",
		},
		{
			name: "networkSecurityGroupID change should fail",
			modify: func(cp *AROControlPlane) {
				cp.Spec.Platform.NetworkSecurityGroupID = "different-nsg"
			},
			expectError: true,
			errorField:  "spec.platform.networkSecurityGroupID",
		},
		{
			name: "no changes should pass",
			modify: func(cp *AROControlPlane) {
				// No changes
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create a copy of the base control plane
			old := baseControlPlane.DeepCopy()
			new := baseControlPlane.DeepCopy()

			// Apply the modification
			tc.modify(new)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			webhook := &aroControlPlaneWebhook{Client: fakeClient}

			_, err := webhook.ValidateUpdate(context.TODO(), old, new)

			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
				if tc.errorField != "" {
					g.Expect(err.Error()).To(ContainSubstring(tc.errorField))
				}
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
