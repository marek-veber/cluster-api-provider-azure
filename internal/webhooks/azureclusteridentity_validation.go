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
	"fmt"
	"path/filepath"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

var allowedCertDirs = []string{"/var/run/secrets/azure/"}

func validateAzureClusterIdentity(c *infrav1.AzureClusterIdentity) (admission.Warnings, error) {
	var allErrs field.ErrorList
	if c.Spec.Type != infrav1.UserAssignedMSI && c.Spec.ResourceID != "" { //nolint:staticcheck
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "resourceID"), c.Spec.ResourceID)) //nolint:staticcheck
	}
	if c.Spec.Type != infrav1.UserAssignedIdentityCredential && (c.Spec.UserAssignedIdentityCredentialsPath != "" || c.Spec.UserAssignedIdentityCredentialsCloudType != "") {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "userAssignedIdentityCredentialsPath"), fmt.Sprintf("%s can only be set when AzureClusterIdentity is of type UserAssignedIdentityCredential", c.Spec.UserAssignedIdentityCredentialsPath)))
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "userAssignedIdentityCredentialsCloudType"), fmt.Sprintf("%s can only be set when AzureClusterIdentity is of type UserAssignedIdentityCredential ", c.Spec.UserAssignedIdentityCredentialsCloudType)))
	}
	if c.Spec.CertPath != "" {
		if !isPathAllowed(c.Spec.CertPath) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "certPath"),
				"certPath must reference a file under an operator-managed mount (/var/run/secrets/azure/)"))
		}
	}
	if c.Spec.UserAssignedIdentityCredentialsPath != "" {
		if !isPathAllowed(c.Spec.UserAssignedIdentityCredentialsPath) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "userAssignedIdentityCredentialsPath"),
				"userAssignedIdentityCredentialsPath must reference a file under an operator-managed mount (/var/run/secrets/azure/)"))
		}
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind(infrav1.AzureClusterIdentityKind).GroupKind(), c.Name, allErrs)
}

func isPathAllowed(path string) bool {
	clean := filepath.Clean(path)
	for _, dir := range allowedCertDirs {
		if strings.HasPrefix(clean, dir) {
			return true
		}
	}
	return false
}
