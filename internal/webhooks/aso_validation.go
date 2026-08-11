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
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var allowedASOGVKs = map[string]bool{
	"resources.azure.com/ResourceGroup":                   true,
	"containerservice.azure.com/ManagedCluster":           true,
	"containerservice.azure.com/ManagedClustersAgentPool": true,
	"network.azure.com/VirtualNetwork":                    true,
	"network.azure.com/VirtualNetworksSubnet":             true,
}

func validateASOResourceGVKs(resources []runtime.RawExtension, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for i, res := range resources {
		var obj struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
		}
		if err := json.Unmarshal(res.Raw, &obj); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Index(i),
				string(res.Raw),
				fmt.Sprintf("failed to parse resource: %v", err),
			))
			continue
		}
		gk := extractGroupKind(obj.APIVersion, obj.Kind)
		if !allowedASOGVKs[gk] {
			allErrs = append(allErrs, field.Forbidden(
				fldPath.Index(i),
				fmt.Sprintf("resource type %q is not allowed; allowed types: %s", gk, allowedASOGVKList()),
			))
		}
	}
	return allErrs
}

func extractGroupKind(apiVersion, kind string) string {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) < 2 {
		return apiVersion + "/" + kind
	}
	return parts[0] + "/" + kind
}

func allowedASOGVKList() string {
	keys := make([]string, 0, len(allowedASOGVKs))
	for k := range allowedASOGVKs {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
