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
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	azureutil "sigs.k8s.io/cluster-api-provider-azure/util/azure"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// UserAssignedIdentitySpec defines the specification for a user-assigned identity.
type UserAssignedIdentitySpec struct {
	Name          string
	ResourceGroup string
	Location      string
	Tags          map[string]*string
}

// ResourceName returns the name of the user-assigned identity.
func (s *UserAssignedIdentitySpec) ResourceName() string {
	return s.Name
}

// ResourceGroupName returns the resource group name.
func (s *UserAssignedIdentitySpec) ResourceGroupName() string {
	return s.ResourceGroup
}

// OwnerResourceName is a no-op for user-assigned identities.
func (s *UserAssignedIdentitySpec) OwnerResourceName() string {
	return ""
}

// Parameters returns the parameters for the user-assigned identity.
func (s *UserAssignedIdentitySpec) Parameters(ctx context.Context, existing interface{}) (params interface{}, err error) {
	_, log, done := tele.StartSpanWithLogger(ctx, "identities.UserAssignedIdentitySpec.Parameters")
	defer done()

	if existing != nil {
		if _, ok := existing.(armmsi.Identity); !ok {
			return nil, errors.Errorf("expected existing user-assigned identity, got %T", existing)
		}
		log.V(2).Info("user-assigned identity already exists")
		return nil, nil
	}

	return armmsi.Identity{
		Location: ptr.To(s.Location),
		Tags:     s.Tags,
	}, nil
}

// GetResourceID constructs the resource ID for the user-assigned identity.
func (s *UserAssignedIdentitySpec) GetResourceID(subscriptionID string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
		subscriptionID, s.ResourceGroup, s.Name)
}

// ParseUserAssignedIdentityResourceID parses a user-assigned identity resource ID and returns the parsed components.
func ParseUserAssignedIdentityResourceID(resourceID string) (*UserAssignedIdentitySpec, error) {
	parsed, err := azureutil.ParseResourceID(resourceID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse resource ID")
	}

	// The Azure SDK puts the full resource type in ResourceType field as "Microsoft.ManagedIdentity/userAssignedIdentities"
	// We need to split this to get the provider and resource type separately
	resourceTypeString := parsed.ResourceType.String()
	if resourceTypeString != "Microsoft.ManagedIdentity/userAssignedIdentities" {
		return nil, errors.Errorf("expected resource type Microsoft.ManagedIdentity/userAssignedIdentities, got %s", resourceTypeString)
	}

	return &UserAssignedIdentitySpec{
		Name:          parsed.Name,
		ResourceGroup: parsed.ResourceGroupName,
	}, nil
}

// ValidateCreate validates the user-assigned identity spec for creation.
func (s *UserAssignedIdentitySpec) ValidateCreate() error {
	if s.Name == "" {
		return errors.New("user-assigned identity name cannot be empty")
	}
	if s.ResourceGroup == "" {
		return errors.New("resource group name cannot be empty")
	}
	if s.Location == "" {
		return errors.New("location cannot be empty")
	}
	return nil
}

// ValidateUpdate validates the user-assigned identity spec for update.
func (s *UserAssignedIdentitySpec) ValidateUpdate(existing interface{}) error {
	if existing == nil {
		return s.ValidateCreate()
	}

	existingIdentity, ok := existing.(armmsi.Identity)
	if !ok {
		return errors.Errorf("expected existing user-assigned identity, got %T", existing)
	}

	// Location is immutable
	if existingIdentity.Location != nil && *existingIdentity.Location != s.Location {
		return errors.Errorf("location cannot be changed from %s to %s", *existingIdentity.Location, s.Location)
	}

	return nil
}
