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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/hcpopenshiftclusters"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/identities"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

const serviceName = "hcpopenshiftidentities"

// HcpOpenShiftIdentityScope defines the scope interface for HCP OpenShift identity service.
type HcpOpenShiftIdentityScope interface {
	azure.Authorizer
	azure.AsyncStatusUpdater
	HcpOpenShiftClusterSpecs(context.Context) azure.ResourceSpecGetter
}

// Service provides operations on Azure user-assigned identities for HCP OpenShift clusters.
type Service struct {
	Scope            HcpOpenShiftIdentityScope
	identitiesGetter identities.Client
}

// New creates a new service.
func New(scope HcpOpenShiftIdentityScope, identitiesGetter identities.Client) (*Service, error) {
	if scope == nil {
		return nil, errors.New("scope cannot be nil")
	}
	if identitiesGetter == nil {
		return nil, errors.New("identitiesGetter cannot be nil")
	}

	return &Service{
		Scope:            scope,
		identitiesGetter: identitiesGetter,
	}, nil
}

// Name returns the service name.
func (s *Service) Name() string {
	return serviceName
}

// Reconcile ensures that all required user-assigned identities exist for the HCP OpenShift cluster.
func (s *Service) Reconcile(ctx context.Context) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "hcpopenshiftidentities.Service.Reconcile")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, s.Scope.DefaultedAzureServiceReconcileTimeout())
	defer cancel()

	spec := s.Scope.HcpOpenShiftClusterSpecs(ctx)
	hcpOpenShiftClusterSpecs, ok := spec.(*hcpopenshiftclusters.HcpOpenShiftClustersSpec)
	if !ok {
		return errors.Errorf("%T is not of type HcpOpenShiftClustersSpec", spec)
	}

	err := s.ensureManagedIdentities(ctx, log, hcpOpenShiftClusterSpecs)
	if err != nil {
		s.Scope.UpdatePutStatus(infrav1.BootstrapSucceededCondition, serviceName, err)
		return errors.Wrap(err, "failed to ensure managed identities")
	}

	s.Scope.UpdatePutStatus(infrav1.BootstrapSucceededCondition, serviceName, nil)
	return nil
}

// Delete is a no-op for identities as they are managed separately.
func (s *Service) Delete(ctx context.Context) error {
	_, _, done := tele.StartSpanWithLogger(ctx, "hcpopenshiftidentities.Service.Delete")
	defer done()

	// Identities are not deleted as part of cluster deletion
	// They may be shared across multiple clusters or managed separately
	return nil
}

// IsManaged always returns true as we need to ensure identities exist.
func (s *Service) IsManaged(_ context.Context) (bool, error) {
	return true, nil
}

// ensureManagedIdentities ensures that all required user-assigned identities exist.
// If CreateAROHCPManagedIdentities is enabled, missing identities will be created automatically.
// If disabled, it will only validate that all identities exist.
func (s *Service) ensureManagedIdentities(ctx context.Context, log logr.Logger, spec *hcpopenshiftclusters.HcpOpenShiftClustersSpec) error {
	userAssignedIdentities, _ := spec.GetManagedIdentities()

	// Collect all identity resource IDs
	var identityResourceIDs []string

	// Add control plane operator identities
	for _, mid := range userAssignedIdentities.ControlPlaneOperators {
		if mid != nil && *mid != "" {
			identityResourceIDs = append(identityResourceIDs, *mid)
		}
	}

	// Add data plane operator identities
	for _, mid := range userAssignedIdentities.DataPlaneOperators {
		if mid != nil && *mid != "" {
			identityResourceIDs = append(identityResourceIDs, *mid)
		}
	}

	// Add service managed identity
	if userAssignedIdentities.ServiceManagedIdentity != nil && *userAssignedIdentities.ServiceManagedIdentity != "" {
		identityResourceIDs = append(identityResourceIDs, *userAssignedIdentities.ServiceManagedIdentity)
	}

	if len(identityResourceIDs) == 0 {
		log.V(4).Info("no managed identities specified")
		return nil
	}

	createIfMissing := spec.ManagedIdentities.CreateAROHCPManagedIdentities
	if createIfMissing {
		log.V(2).Info("ensuring user-assigned identities exist (will create if missing)", "count", len(identityResourceIDs))
	} else {
		log.V(2).Info("checking user-assigned identities exist (validation only)", "count", len(identityResourceIDs))
	}

	// Ensure identities exist (create if missing and enabled, or validate if disabled)
	if err := identities.EnsureUserAssignedIdentities(
		ctx,
		s.identitiesGetter,
		identityResourceIDs,
		spec.ResourceGroup,
		spec.Location,
		buildIdentityTags(spec.AdditionalTags),
		createIfMissing,
	); err != nil {
		return errors.Wrap(err, "failed to ensure user-assigned identities")
	}

	if createIfMissing {
		log.V(2).Info("successfully ensured all user-assigned identities exist")
	} else {
		log.V(2).Info("successfully validated all user-assigned identities exist")
	}
	return nil
}

// buildIdentityTags creates tags for user-assigned identities.
func buildIdentityTags(additionalTags map[string]string) map[string]*string {
	tags := make(map[string]*string)

	// Add default tags
	tags["created-by"] = ptr.To("cluster-api-provider-azure")
	tags["purpose"] = ptr.To("aro-hcp-managed-identity")

	// Add additional tags
	for key, value := range additionalTags {
		tags[key] = ptr.To(value)
	}

	return tags
}
