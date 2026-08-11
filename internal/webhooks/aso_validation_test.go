/*
Copyright The Kubernetes Authors.

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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateASOResourceGVKs(t *testing.T) {
	tests := []struct {
		name      string
		resources []runtime.RawExtension
		wantErrs  int
	}{
		{
			name:      "empty resources",
			resources: nil,
			wantErrs:  0,
		},
		{
			name: "allowed ResourceGroup",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"resources.azure.com/v1api20200601","kind":"ResourceGroup"}`)},
			},
			wantErrs: 0,
		},
		{
			name: "allowed ManagedCluster",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"containerservice.azure.com/v1api20231001","kind":"ManagedCluster"}`)},
			},
			wantErrs: 0,
		},
		{
			name: "allowed ManagedClustersAgentPool",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"containerservice.azure.com/v1api20231001","kind":"ManagedClustersAgentPool"}`)},
			},
			wantErrs: 0,
		},
		{
			name: "allowed VirtualNetwork",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"network.azure.com/v1api20201101","kind":"VirtualNetwork"}`)},
			},
			wantErrs: 0,
		},
		{
			name: "allowed VirtualNetworksSubnet",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"network.azure.com/v1api20201101","kind":"VirtualNetworksSubnet"}`)},
			},
			wantErrs: 0,
		},
		{
			name: "disallowed type",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"compute.azure.com/v1api20201201","kind":"VirtualMachine"}`)},
			},
			wantErrs: 1,
		},
		{
			name: "mixed allowed and disallowed",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{"apiVersion":"resources.azure.com/v1api20200601","kind":"ResourceGroup"}`)},
				{Raw: []byte(`{"apiVersion":"compute.azure.com/v1api20201201","kind":"VirtualMachine"}`)},
			},
			wantErrs: 1,
		},
		{
			name: "invalid JSON",
			resources: []runtime.RawExtension{
				{Raw: []byte(`{not valid json}`)},
			},
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			errs := validateASOResourceGVKs(tt.resources, field.NewPath("spec", "resources"))
			g.Expect(errs).To(HaveLen(tt.wantErrs))
		})
	}
}

func TestExtractGroupKind(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		kind       string
		want       string
	}{
		{
			name:       "standard group/version",
			apiVersion: "resources.azure.com/v1api20200601",
			kind:       "ResourceGroup",
			want:       "resources.azure.com/ResourceGroup",
		},
		{
			name:       "no version separator",
			apiVersion: "v1",
			kind:       "ConfigMap",
			want:       "v1/ConfigMap",
		},
		{
			name:       "containerservice group",
			apiVersion: "containerservice.azure.com/v1api20231001",
			kind:       "ManagedCluster",
			want:       "containerservice.azure.com/ManagedCluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(extractGroupKind(tt.apiVersion, tt.kind)).To(Equal(tt.want))
		})
	}
}
