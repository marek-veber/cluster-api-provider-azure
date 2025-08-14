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

	"github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api-provider-azure/azure/services/identities"
)

func TestNew(t *testing.T) {
	testCases := []struct {
		name           string
		scope          HcpOpenShiftIdentityScope
		identityGetter identities.Client
		expectError    bool
	}{
		{
			name:           "nil scope",
			scope:          nil,
			identityGetter: nil,
			expectError:    true,
		},
		{
			name:           "nil identity getter",
			scope:          nil,
			identityGetter: nil,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			service, err := New(tc.scope, tc.identityGetter)

			if tc.expectError {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(service).To(gomega.BeNil())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(service).NotTo(gomega.BeNil())
				g.Expect(service.Name()).To(gomega.Equal("hcpopenshiftidentities"))
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	g := gomega.NewWithT(t)

	service := &Service{}
	err := service.Delete(context.TODO())

	// Delete should always succeed and be a no-op
	g.Expect(err).NotTo(gomega.HaveOccurred())
}

func TestService_IsManaged(t *testing.T) {
	g := gomega.NewWithT(t)

	service := &Service{}
	managed, err := service.IsManaged(context.TODO())

	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(managed).To(gomega.BeTrue())
}

func TestService_Name(t *testing.T) {
	g := gomega.NewWithT(t)

	service := &Service{}
	name := service.Name()

	g.Expect(name).To(gomega.Equal("hcpopenshiftidentities"))
}
