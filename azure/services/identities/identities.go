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
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// EnsureUserAssignedIdentities ensures that all required user-assigned identities exist.
// This function checks for the existence of identities and optionally creates them if they don't exist.
// If createIfMissing is false, it will only validate that identities exist and return an error if any are missing.
func EnsureUserAssignedIdentities(ctx context.Context, client Client, identities []string, _, location string, tags map[string]*string, createIfMissing bool) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "identities.EnsureUserAssignedIdentities")
	defer done()

	for _, identityResourceID := range identities {
		if identityResourceID == "" {
			continue
		}

		// Parse the resource ID to get the identity name and resource group
		spec, err := ParseUserAssignedIdentityResourceID(identityResourceID)
		if err != nil {
			return errors.Wrapf(err, "failed to parse identity resource ID %s", identityResourceID)
		}

		// Check if the identity exists
		_, err = client.Get(ctx, spec.ResourceGroup, spec.Name)
		if err == nil {
			log.V(4).Info("user-assigned identity already exists", "identity", spec.Name)
			continue
		}

		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to check existence of identity %s", spec.Name)
		}

		// Identity doesn't exist
		if !createIfMissing {
			return errors.Errorf("user-assigned identity %s does not exist and createIfMissing is disabled", spec.Name)
		}

		// Create the missing identity
		log.V(2).Info("creating missing user-assigned identity", "identity", spec.Name, "resourceGroup", spec.ResourceGroup)

		identity := armmsi.Identity{
			Location: &location,
			Tags:     tags,
		}

		_, err = client.CreateOrUpdate(ctx, spec.ResourceGroup, spec.Name, identity)
		if err != nil {
			return errors.Wrapf(err, "failed to create identity %s", spec.Name)
		}

		log.V(2).Info("successfully created user-assigned identity",
			"identity", spec.Name,
			"resourceGroup", spec.ResourceGroup)
	}

	return nil
}
