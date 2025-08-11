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

package hcpopenshiftidentities

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"sigs.k8s.io/cluster-api-provider-azure/azure/services/identities/mock_identities"
)

func TestNew(t *testing.T) {
	testCases := []struct {
		name        string
		scope       HcpOpenShiftIdentityScope
		expectError bool
	}{
		{
			name:        "nil scope",
			scope:       nil,
			expectError: true,
		},
		{
			name:        "nil identity getter",
			scope:       nil, // Will test with nil identityGetter
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			var identityGetter *mock_identities.MockClient
			if tc.name == "nil identity getter" {
				identityGetter = nil
			} else {
				identityGetter = mock_identities.NewMockClient(mockCtrl)
			}

			service, err := New(tc.scope, identityGetter)

			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(service).To(BeNil())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(service).NotTo(BeNil())
				g.Expect(service.Name()).To(Equal("hcpopenshiftidentities"))
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	g := NewWithT(t)

	service := &Service{}
	err := service.Delete(context.TODO())

	// Delete should always succeed and be a no-op
	g.Expect(err).NotTo(HaveOccurred())
}

func TestService_IsManaged(t *testing.T) {
	g := NewWithT(t)

	service := &Service{}
	managed, err := service.IsManaged(context.TODO())

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(managed).To(BeTrue())
}

func TestService_Name(t *testing.T) {
	g := NewWithT(t)

	service := &Service{}
	name := service.Name()

	g.Expect(name).To(Equal("hcpopenshiftidentities"))
}
