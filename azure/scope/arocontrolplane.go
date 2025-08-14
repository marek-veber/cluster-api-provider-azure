/*
Copyright 2018 The Kubernetes Authors.

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

package scope

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	asonetworkv1api20201101 "github.com/Azure/azure-service-operator/v2/api/network/v1api20201101"
	asoresourcesv1 "github.com/Azure/azure-service-operator/v2/api/resources/v1api20200601"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/groups"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/hcpopenshiftclustercredentials"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/hcpopenshiftclusters"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/identities"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/roleassignments"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/securitygroups"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/subnets"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/virtualnetworks"
	cplane "sigs.k8s.io/cluster-api-provider-azure/exp/api/controlplane/v1beta2"
	arohcp "sigs.k8s.io/cluster-api-provider-azure/exp/third_party/aro-hcp/api/v20240610preview/generated"
	"sigs.k8s.io/cluster-api-provider-azure/util/futures"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

const (
	kubeconfigRefreshNeededValue = "true"
)

// Role definition IDs for ARO HCP cluster role assignments.
type roleDEF string

const (
	roleDEFhcpClusterAPIProvider     = roleDEF("88366f10-ed47-4cc0-9fab-c8a06148393e")
	roleDEFreader                    = roleDEF("acdd72a7-3385-48ef-bd42-f606fba81ae7")
	roleDEFhcpControlPlaneOperator   = roleDEF("fc0c873f-45e9-4d0d-a7d1-585aab30c6ed")
	roleDEFcloudControllerManager    = roleDEF("a1f96423-95ce-4224-ab27-4e3dc72facd4")
	roleDEFingressOperator           = roleDEF("0336e1d3-7a87-462b-b6db-342b63f7802c")
	roleDEFfileStorageOperator       = roleDEF("0d7aedc0-15fd-4a67-a412-efad370c947e")
	roleDEFnetworkOperator           = roleDEF("be7a6435-15ae-4171-8f30-4a343eff9e8f")
	roleDEFfederatedCredentials      = roleDEF("ef318e2a-8334-4a05-9e4a-295a196c6a6e")
	roleDEFhcpServiceManagedIdentity = roleDEF("c0ff367d-66d8-445e-917c-583feb0ef0d4")
)

// AROControlPlaneScopeParams defines the input parameters used to create a new Scope.
type AROControlPlaneScopeParams struct {
	AzureClients
	Client          client.Client
	Cluster         *clusterv1.Cluster
	ControlPlane    *cplane.AROControlPlane
	Cache           *AROControlPlaneCache
	Timeouts        azure.AsyncReconciler
	CredentialCache azure.CredentialCache
}

// NewAROControlPlaneScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewAROControlPlaneScope(ctx context.Context, params AROControlPlaneScopeParams) (*AROControlPlaneScope, error) {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "azure.aroControlPlaneScope.NewAROControlPlaneScope")
	defer done()

	if params.ControlPlane == nil {
		return nil, errors.New("failed to generate new scope from nil AROControlPlane")
	}

	credentialsProvider, err := NewAzureCredentialsProvider(ctx, params.CredentialCache, params.Client, params.ControlPlane.Spec.IdentityRef, params.ControlPlane.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init credentials provider")
	}
	err = params.AzureClients.setCredentialsWithProvider(ctx, params.ControlPlane.Spec.SubscriptionID, params.ControlPlane.Spec.AzureEnvironment, credentialsProvider)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure azure settings and credentials for Identity")
	}

	if params.Cache == nil {
		params.Cache = &AROControlPlaneCache{}
	}

	helper, err := patch.NewHelper(params.ControlPlane, params.Client)
	if err != nil {
		return nil, errors.Errorf("failed to init patch helper: %v", err)
	}

	scope := &AROControlPlaneScope{
		Client:          params.Client,
		AzureClients:    params.AzureClients,
		Cluster:         params.Cluster,
		ControlPlane:    params.ControlPlane,
		patchHelper:     helper,
		cache:           params.Cache,
		AsyncReconciler: params.Timeouts,
	}
	scope.initNetworkSpec()

	return scope, nil
}

// AROControlPlaneScope defines the basic context for an actuator to operate upon.
type AROControlPlaneScope struct {
	Client      client.Client
	patchHelper *patch.Helper
	cache       *AROControlPlaneCache

	AzureClients
	Cluster              *clusterv1.Cluster
	ControlPlane         *cplane.AROControlPlane
	ControlPlaneEndpoint clusterv1.APIEndpoint

	NetworkSpec *infrav1.NetworkSpec

	Kubeconfig                   *string
	KubeonfigExpirationTimestamp *time.Time
	azure.AsyncReconciler
}

// SetAPIURL sets the API URL and visibility for the ARO control plane.
func (s *AROControlPlaneScope) SetAPIURL(url *string, _ *arohcp.Visibility) {
	if url != nil {
		s.ControlPlane.Status.APIURL = *url
	}
}

// SetKubeconfig sets the kubeconfig data and expiration timestamp.
func (s *AROControlPlaneScope) SetKubeconfig(kubeconfig *string, kubeconfigExpirationTimestamp *time.Time) {
	s.Kubeconfig = kubeconfig
	s.KubeonfigExpirationTimestamp = kubeconfigExpirationTimestamp
}

// GetAdminKubeconfigData returns the admin kubeconfig data as bytes.
func (s *AROControlPlaneScope) GetAdminKubeconfigData() []byte {
	if s.Kubeconfig == nil {
		return nil
	}
	return []byte(*s.Kubeconfig)
}

// MakeEmptyKubeConfigSecret creates an empty secret object that is used for storing kubeconfig secret data.
func (s *AROControlPlaneScope) MakeEmptyKubeConfigSecret() corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name(s.Cluster.Name, secret.Kubeconfig),
			Namespace: s.Cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(s.ControlPlane, infrav1.GroupVersion.WithKind(cplane.AROControlPlaneKind)),
			},
			Labels: map[string]string{clusterv1.ClusterNameLabel: s.Cluster.Name},
		},
	}
}

// SetStatusVersion sets the version profile in the control plane status.
func (s *AROControlPlaneScope) SetStatusVersion(version *arohcp.VersionProfile) {
	if version == nil {
		return
	}
	if version.ID != nil {
		s.ControlPlane.Status.Version = *version.ID
	}
}

// SetProvisioningState sets the provisioning state in the control plane status.
func (s *AROControlPlaneScope) SetProvisioningState(state *arohcp.ProvisioningState) {
	if state == nil {
		conditions.MarkUnknown(s.ControlPlane, cplane.AROControlPlaneReadyCondition, infrav1.CreatingReason, "nil ProvisioningState was returned")
		return
	}
	if *state == arohcp.ProvisioningStateSucceeded {
		conditions.MarkTrue(s.ControlPlane, cplane.AROControlPlaneReadyCondition)
		return
	}
	conditions.MarkFalse(s.ControlPlane, cplane.AROControlPlaneReadyCondition, infrav1.CreatingReason, clusterv1.ConditionSeverityInfo, "ProvisioningState=%s", string(*state))
}

// SetLongRunningOperationState will set the future on the AROControlPlane status to allow the resource to continue
// in the next reconciliation.
func (s *AROControlPlaneScope) SetLongRunningOperationState(future *infrav1.Future) {
	futures.Set(s.ControlPlane, future)
}

// GetLongRunningOperationState will get the future on the AROControlPlane status.
func (s *AROControlPlaneScope) GetLongRunningOperationState(name, service, futureType string) *infrav1.Future {
	return futures.Get(s.ControlPlane, name, service, futureType)
}

// DeleteLongRunningOperationState will delete the future from the AROControlPlane status.
func (s *AROControlPlaneScope) DeleteLongRunningOperationState(name, service, futureType string) {
	futures.Delete(s.ControlPlane, name, service, futureType)
}

// UpdateDeleteStatus updates a condition on the AROControlPlane status after a DELETE operation.
func (s *AROControlPlaneScope) UpdateDeleteStatus(condition clusterv1.ConditionType, service string, err error) {
	switch {
	case err == nil:
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.DeletedReason, clusterv1.ConditionSeverityInfo, "%s successfully deleted", service)
	case azure.IsOperationNotDoneError(err):
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.DeletingReason, clusterv1.ConditionSeverityInfo, "%s deleting", service)
	default:
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.DeletionFailedReason, clusterv1.ConditionSeverityError, "%s failed to delete. err: %s", service, err.Error())
	}
}

// UpdatePutStatus updates a condition on the AROControlPlane status after a PUT operation.
func (s *AROControlPlaneScope) UpdatePutStatus(condition clusterv1.ConditionType, service string, err error) {
	switch {
	case err == nil:
		conditions.MarkTrue(s.ControlPlane, condition)
	case azure.IsOperationNotDoneError(err):
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.CreatingReason, clusterv1.ConditionSeverityInfo, "%s creating or updating", service)
	default:
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.FailedReason, clusterv1.ConditionSeverityError, "%s failed to create or update. err: %s", service, err.Error())
	}
}

// UpdatePatchStatus updates a condition on the AROControlPlane status after a PATCH operation.
func (s *AROControlPlaneScope) UpdatePatchStatus(condition clusterv1.ConditionType, service string, err error) {
	switch {
	case err == nil:
		conditions.MarkTrue(s.ControlPlane, condition)
	case azure.IsOperationNotDoneError(err):
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.UpdatingReason, clusterv1.ConditionSeverityInfo, "%s updating", service)
	default:
		conditions.MarkFalse(s.ControlPlane, condition, infrav1.FailedReason, clusterv1.ConditionSeverityError, "%s failed to update. err: %s", service, err.Error())
	}
}

// HcpOpenShiftClusterSpecs returns the resource spec getter for HCP OpenShift clusters.
func (s *AROControlPlaneScope) HcpOpenShiftClusterSpecs(_ context.Context) azure.ResourceSpecGetter {
	ret := &hcpopenshiftclusters.HcpOpenShiftClustersSpec{
		Name:                   s.Cluster.Name,
		Location:               s.Location(),
		ResourceGroup:          s.ResourceGroup(),
		NodeResourceGroup:      s.NodeResourceGroup(),
		ManagedIdentities:      &s.ControlPlane.Spec.Platform.ManagedIdentities,
		AdditionalTags:         s.ControlPlane.Spec.AdditionalTags,
		NetworkSecurityGroupID: s.ControlPlane.Spec.Platform.NetworkSecurityGroupID,
		SubscriptionID:         s.ControlPlane.Spec.SubscriptionID,
		SubnetID:               s.ControlPlane.Spec.Platform.Subnet,
		VNetID:                 regexp.MustCompile("/subnets/.*").ReplaceAllLiteralString(s.ControlPlane.Spec.Platform.Subnet, ""),
		OutboundType:           s.ControlPlane.Spec.Platform.OutboundType,
		Network:                s.ControlPlane.Spec.Network,
		Version:                s.ControlPlane.Spec.Version,
		ChannelGroup:           s.ControlPlane.Spec.ChannelGroup,
		Visibility:             s.ControlPlane.Spec.Visibility,
	}
	return ret
}

// HcpOpenShiftClusterCredentialsSpecs returns the resource spec getter for HCP OpenShift cluster credentials.
func (s *AROControlPlaneScope) HcpOpenShiftClusterCredentialsSpecs(_ context.Context) azure.ResourceSpecGetter {
	ret := &hcpopenshiftclustercredentials.HcpOpenShiftClusterCredentialsSpec{
		Name:          s.Cluster.Name,
		ResourceGroup: s.ResourceGroup(),
		APIURI:        s.ControlPlane.Status.APIURL,
	}
	return ret
}

// AnnotateKubeconfigInvalid adds annotation aro.azure.com/kubeconfig-refresh-needed: true.
// This marks this secret as invalid.
func (s *AROControlPlaneScope) AnnotateKubeconfigInvalid(ctx context.Context) error {
	kubeconfigSecret := s.MakeEmptyKubeConfigSecret()
	key := client.ObjectKeyFromObject(&kubeconfigSecret)
	if err := s.Client.Get(ctx, key, &kubeconfigSecret); err != nil {
		// Secret doesn't exist - there is no need to invalidate it
		return nil //nolint:nilerr // returning nil when secret doesn't exist is intentional
	}
	// Update the kubeconfig secret
	kubeConfigSecret := s.MakeEmptyKubeConfigSecret()
	if _, err := controllerutil.CreateOrUpdate(ctx, s.Client, &kubeConfigSecret, func() error {
		// Add annotations for tracking
		if kubeConfigSecret.Annotations == nil {
			kubeConfigSecret.Annotations = make(map[string]string)
		}
		kubeConfigSecret.Annotations["aro.azure.com/kubeconfig-refresh-needed"] = kubeconfigRefreshNeededValue
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to invalidate kubeconfig secret")
	}
	return nil
}

// ShouldReconcileKubeconfig determines if kubeconfig needs reconciliation using metadata-based validation (Pattern 3).
// This avoids direct cluster connections and prevents issues with stale/invalid secrets.
func (s *AROControlPlaneScope) ShouldReconcileKubeconfig(ctx context.Context) bool {
	kubeconfigSecret := s.MakeEmptyKubeConfigSecret()
	key := client.ObjectKeyFromObject(&kubeconfigSecret)

	if err := s.Client.Get(ctx, key, &kubeconfigSecret); err != nil {
		// Secret doesn't exist - need to create it
		return true
	}

	// Check if kubeconfig data exists
	if len(kubeconfigSecret.Data[secret.KubeconfigDataName]) == 0 {
		return true
	}

	// Check for ARO-specific annotations that indicate refresh needed
	if kubeconfigSecret.Annotations != nil {
		if refreshNeeded, exists := kubeconfigSecret.Annotations["aro.azure.com/kubeconfig-refresh-needed"]; exists && refreshNeeded == kubeconfigRefreshNeededValue {
			return true
		}

		// Check if secret is older than configured threshold
		if lastUpdated, exists := kubeconfigSecret.Annotations["aro.azure.com/kubeconfig-last-updated"]; exists {
			lastUpdatedTime, err := time.Parse(time.RFC3339, lastUpdated)
			if err == nil {
				kubeconfigAge := time.Since(lastUpdatedTime)
				maxAge := s.GetKubeconfigMaxAge() // Configure based on ARO token lifetime
				if kubeconfigAge > maxAge {
					return true
				}
			}
		}
	}

	// Check if we have token expiration information and it's expired
	if s.KubeonfigExpirationTimestamp != nil {
		if time.Now().After(*s.KubeonfigExpirationTimestamp) {
			return true
		}
	}

	return false
}

// GetKubeconfigMaxAge returns the maximum age for kubeconfig before refresh is needed.
func (s *AROControlPlaneScope) GetKubeconfigMaxAge() time.Duration {
	// Default to 30 minutes, but could be made configurable via ControlPlane spec
	return 60 * time.Minute
}

// AROControlPlaneCache stores AROControlPlaneCache data locally so we don't have to hit the API multiple times within the same reconcile loop.
type AROControlPlaneCache struct {
	isVnetManaged *bool
}

// BaseURI returns the Azure ResourceManagerEndpoint.
func (s *AROControlPlaneScope) BaseURI() string {
	return s.ResourceManagerEndpoint
}

// GetClient returns the controller-runtime client.
func (s *AROControlPlaneScope) GetClient() client.Client {
	return s.Client
}

// GetDeletionTimestamp returns the deletion timestamp of the Cluster.
func (s *AROControlPlaneScope) GetDeletionTimestamp() *metav1.Time {
	return s.Cluster.DeletionTimestamp
}

// PatchObject persists the control plane configuration and status.
func (s *AROControlPlaneScope) PatchObject(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "scope.ManagedControlPlaneScope.PatchObject")
	defer done()

	conditions.SetSummary(s.ControlPlane)

	return s.patchHelper.Patch(
		ctx,
		s.ControlPlane,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			cplane.AROControlPlaneReadyCondition,
			cplane.AROControlPlaneValidCondition,
			cplane.AROControlPlaneUpgradingCondition,
		}})
}

// Close closes the current scope persisting the control plane configuration and status.
func (s *AROControlPlaneScope) Close(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "scope.AROControlPlaneScope.Close")
	defer done()

	return s.PatchObject(ctx)
}

// Location returns location.
func (s *AROControlPlaneScope) Location() string {
	return s.ControlPlane.Spec.Platform.Location
}

// SetVersionStatus sets the k8s version in status.
func (s *AROControlPlaneScope) SetVersionStatus(version string) {
	s.ControlPlane.Status.Version = version
}

// MakeClusterCA returns a cluster CA Secret for the managed control plane.
func (s *AROControlPlaneScope) MakeClusterCA() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name(s.Cluster.Name, secret.ClusterCA),
			Namespace: s.Cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(s.ControlPlane, cplane.GroupVersion.WithKind(cplane.AROControlPlaneKind)),
			},
		},
	}
}

// StoreClusterInfo stores the discovery cluster-info configmap in the kube-public namespace on the AKS cluster so kubeadm can access it to join nodes.
// This method now avoids direct cluster connections to prevent reliability issues with stale kubeconfigs.
func (s *AROControlPlaneScope) StoreClusterInfo(_ context.Context, _ []byte) error {
	// Skip cluster-info creation if we don't have a valid control plane endpoint
	// This avoids the need for remote cluster connections during kubeconfig reconciliation
	if s.ControlPlaneEndpoint.Host == "" || s.ControlPlaneEndpoint.Port == 0 {
		// Log that we're skipping this step but don't fail the reconciliation
		// The cluster-info will be created when the control plane is ready
		return nil
	}

	// For ARO clusters, we typically don't need to create cluster-info configmaps
	// as ARO manages this internally. This method is kept for compatibility
	// but we avoid remote connections to prevent kubeconfig validation issues.

	// For now, we skip this step to avoid the reliability issues with remote cluster connections

	return nil
}

// ASOOwner implements aso.Scope.
func (s *AROControlPlaneScope) ASOOwner() client.Object {
	return s.ControlPlane
}

// NSGSpecs returns the security group specs.
func (s *AROControlPlaneScope) NSGSpecs() []azure.ResourceSpecGetter {
	nsgspecs := make([]azure.ResourceSpecGetter, len(s.NetworkSpec.Subnets))
	for i, subnet := range s.NetworkSpec.Subnets {
		nsgspecs[i] = &securitygroups.NSGSpec{
			Name:                     subnet.SecurityGroup.Name,
			SecurityRules:            subnet.SecurityGroup.SecurityRules,
			ResourceGroup:            s.Vnet().ResourceGroup,
			Location:                 s.Location(),
			ClusterName:              s.ClusterName(),
			AdditionalTags:           s.AdditionalTags(),
			LastAppliedSecurityRules: s.getLastAppliedSecurityRules(subnet.SecurityGroup.Name),
		}
	}

	return nsgspecs
}

// SubnetSpecs returns the subnets specs.
func (s *AROControlPlaneScope) SubnetSpecs() []azure.ASOResourceSpecGetter[*asonetworkv1api20201101.VirtualNetworksSubnet] {
	numberOfSubnets := len(s.NetworkSpec.Subnets)

	subnetSpecs := make([]azure.ASOResourceSpecGetter[*asonetworkv1api20201101.VirtualNetworksSubnet], 0, numberOfSubnets)

	for _, subnet := range s.NetworkSpec.Subnets {
		subnetSpec := &subnets.SubnetSpec{
			Name:              subnet.Name,
			ResourceGroup:     s.ResourceGroup(),
			SubscriptionID:    s.SubscriptionID(),
			CIDRs:             subnet.CIDRBlocks,
			VNetName:          s.Vnet().Name,
			VNetResourceGroup: s.Vnet().ResourceGroup,
			IsVNetManaged:     s.IsVnetManaged(),
			RouteTableName:    subnet.RouteTable.Name,
			SecurityGroupName: subnet.SecurityGroup.Name,
			NatGatewayName:    subnet.NatGateway.Name,
			ServiceEndpoints:  subnet.ServiceEndpoints,
		}
		subnetSpecs = append(subnetSpecs, subnetSpec)
	}

	return subnetSpecs
}

// GroupSpecs returns the resource group spec.
func (s *AROControlPlaneScope) GroupSpecs() []azure.ASOResourceSpecGetter[*asoresourcesv1.ResourceGroup] {
	specs := []azure.ASOResourceSpecGetter[*asoresourcesv1.ResourceGroup]{
		&groups.GroupSpec{
			Name:           s.ResourceGroup(),
			AzureName:      s.ResourceGroup(),
			Location:       s.Location(),
			ClusterName:    s.ClusterName(),
			AdditionalTags: s.AdditionalTags(),
		},
	}
	if s.Vnet().ResourceGroup != "" && s.Vnet().ResourceGroup != s.ResourceGroup() {
		specs = append(specs, &groups.GroupSpec{
			Name:           azure.GetNormalizedKubernetesName(s.Vnet().ResourceGroup),
			AzureName:      s.Vnet().ResourceGroup,
			Location:       s.Location(),
			ClusterName:    s.ClusterName(),
			AdditionalTags: s.AdditionalTags(),
		})
	}
	return specs
}

// VNetSpec returns the virtual network spec.
func (s *AROControlPlaneScope) VNetSpec() azure.ASOResourceSpecGetter[*asonetworkv1api20201101.VirtualNetwork] {
	return &virtualnetworks.VNetSpec{
		ResourceGroup:    s.Vnet().ResourceGroup,
		Name:             s.Vnet().Name,
		CIDRs:            s.Vnet().CIDRBlocks,
		ExtendedLocation: s.ExtendedLocation(),
		Location:         s.Location(),
		ClusterName:      s.ClusterName(),
		AdditionalTags:   s.AdditionalTags(),
	}
}

// Vnet returns the cluster Vnet.
func (s *AROControlPlaneScope) Vnet() *infrav1.VnetSpec {
	return &s.NetworkSpec.Vnet
}

// Subnet returns the subnet with the provided name.
func (s *AROControlPlaneScope) Subnet(name string) infrav1.SubnetSpec {
	for _, sn := range s.NetworkSpec.Subnets {
		if sn.Name == name {
			return sn
		}
	}

	return infrav1.SubnetSpec{}
}

// SetSubnet sets the subnet spec for the subnet with the same name.
func (s *AROControlPlaneScope) SetSubnet(subnetSpec infrav1.SubnetSpec) {
	for i, sn := range s.NetworkSpec.Subnets {
		if sn.Name == subnetSpec.Name {
			s.NetworkSpec.Subnets[i] = subnetSpec
			return
		}
	}
}

// UpdateSubnetCIDRs updates the subnet CIDRs for the subnet with the same name.
func (s *AROControlPlaneScope) UpdateSubnetCIDRs(name string, cidrBlocks []string) {
	subnetSpecInfra := s.Subnet(name)
	subnetSpecInfra.CIDRBlocks = cidrBlocks
	s.SetSubnet(subnetSpecInfra)
}

// UpdateSubnetID updates the subnet ID for the subnet with the same name.
func (s *AROControlPlaneScope) UpdateSubnetID(name string, id string) {
	subnetSpecInfra := s.Subnet(name)
	subnetSpecInfra.ID = id
	s.SetSubnet(subnetSpecInfra)
}

// ResourceGroup returns the cluster resource group.
func (s *AROControlPlaneScope) ResourceGroup() string {
	return s.ControlPlane.Spec.Platform.ResourceGroup
}

// NodeResourceGroup returns the node resource group name for the ARO cluster.
func (s *AROControlPlaneScope) NodeResourceGroup() string {
	return s.ControlPlane.NodeResourceGroup()
}

// ClusterName returns the cluster name.
func (s *AROControlPlaneScope) ClusterName() string {
	return s.Cluster.Name
}

// Namespace returns the cluster namespace.
func (s *AROControlPlaneScope) Namespace() string {
	return s.Cluster.Namespace
}

// AdditionalTags returns AdditionalTags from the scope's AROControlPlane.
func (s *AROControlPlaneScope) AdditionalTags() infrav1.Tags {
	tags := make(infrav1.Tags)
	if s.ControlPlane.Spec.AdditionalTags != nil {
		tags = s.ControlPlane.Spec.AdditionalTags.DeepCopy()
	}
	return tags
}

// ExtendedLocation returns the extended location specification.
func (s *AROControlPlaneScope) ExtendedLocation() *infrav1.ExtendedLocationSpec {
	return nil
}

// IsVnetManaged returns whether the virtual network is managed.
func (s *AROControlPlaneScope) IsVnetManaged() bool {
	if s.cache.isVnetManaged != nil {
		return ptr.Deref(s.cache.isVnetManaged, false)
	}
	// TODO refactor `IsVnetManaged` so that it is able to use an upstream context
	// see https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/2581
	ctx := context.Background()
	ctx, log, done := tele.StartSpanWithLogger(ctx, "scope.ManagedControlPlaneScope.IsVnetManaged")
	defer done()

	vnet := s.VNetSpec().ResourceRef()
	vnet.SetNamespace(s.ASOOwner().GetNamespace())
	err := s.Client.Get(ctx, client.ObjectKeyFromObject(vnet), vnet)
	if err != nil {
		log.Error(err, "Unable to determine if ManagedControlPlaneScope VNET is managed by capz, assuming unmanaged", "AzureManagedCluster", s.ClusterName())
		return false
	}

	isManaged := infrav1.Tags(vnet.Status.Tags).HasOwned(s.ClusterName())
	s.cache.isVnetManaged = ptr.To(isManaged)
	return isManaged
}

func (s *AROControlPlaneScope) getLastAppliedSecurityRules(nsgName string) map[string]interface{} {
	// Retrieve the last applied security rules for all NSGs.
	lastAppliedSecurityRulesAll, err := s.AnnotationJSON(azure.SecurityRuleLastAppliedAnnotation)
	if err != nil {
		return map[string]interface{}{}
	}

	// Retrieve the last applied security rules for this NSG.
	lastAppliedSecurityRules, ok := lastAppliedSecurityRulesAll[nsgName].(map[string]interface{})
	if !ok {
		lastAppliedSecurityRules = map[string]interface{}{}
	}
	return lastAppliedSecurityRules
}

// AnnotationJSON returns a map[string]interface from a JSON annotation.
func (s *AROControlPlaneScope) AnnotationJSON(annotation string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	jsonAnnotation := s.ControlPlane.GetAnnotations()[annotation]
	if jsonAnnotation == "" {
		return out, nil
	}
	err := json.Unmarshal([]byte(jsonAnnotation), &out)
	if err != nil {
		return out, err
	}
	return out, nil
}

// UpdateAnnotationJSON updates the `annotation` with
// `content`. `content` in this case should be a `map[string]interface{}`
// suitable for turning into JSON. This `content` map will be marshalled into a
// JSON string before being set as the given `annotation`.
func (s *AROControlPlaneScope) UpdateAnnotationJSON(annotation string, content map[string]interface{}) error {
	b, err := json.Marshal(content)
	if err != nil {
		return err
	}
	s.SetAnnotation(annotation, string(b))
	return nil
}

// SetAnnotation sets a key value annotation on the ControlPlane.
func (s *AROControlPlaneScope) SetAnnotation(key, value string) {
	if s.ControlPlane.Annotations == nil {
		s.ControlPlane.Annotations = map[string]string{}
	}
	s.ControlPlane.Annotations[key] = value
}

func (s *AROControlPlaneScope) initNetworkSpec() {
	s.NetworkSpec = &infrav1.NetworkSpec{
		Vnet: infrav1.VnetSpec{
			ResourceGroup: s.ControlPlane.Spec.Platform.ResourceGroup,
			ID:            s.vnetID(),
			Name:          s.vnetName(),
		},
		Subnets: infrav1.Subnets{
			infrav1.SubnetSpec{
				SubnetClassSpec: infrav1.SubnetClassSpec{
					Name: s.subnetName(),
				},
				ID: s.ControlPlane.Spec.Platform.Subnet,
				SecurityGroup: infrav1.SecurityGroup{
					ID:   s.ControlPlane.Spec.Platform.NetworkSecurityGroupID,
					Name: s.securityGroupName(),
				},
			},
		},
	}
}

func (s *AROControlPlaneScope) vnetID() string {
	// /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/virtualNetworks/{vnetName}/subnets/{subnetName}
	re := regexp.MustCompile("(/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft.Network/virtualNetworks/[^/]+)/subnets/[^/]+")
	groups := re.FindStringSubmatch(s.ControlPlane.Spec.Platform.Subnet)
	if len(groups) <= 1 {
		return ""
	}
	return groups[1]
}

func (s *AROControlPlaneScope) vnetName() string {
	re := regexp.MustCompile("/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft.Network/virtualNetworks/([^/]+)/subnets/[^/]+")
	groups := re.FindStringSubmatch(s.ControlPlane.Spec.Platform.Subnet)
	if len(groups) <= 1 {
		return ""
	}
	return groups[1]
}
func (s *AROControlPlaneScope) subnetName() string {
	re := regexp.MustCompile("/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft.Network/virtualNetworks/[^/]+/subnets/([^/]+)")
	groups := re.FindStringSubmatch(s.ControlPlane.Spec.Platform.Subnet)
	if len(groups) <= 1 {
		return ""
	}
	return groups[1]
}

func (s *AROControlPlaneScope) securityGroupName() string {
	// /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/networkSecurityGroups/{networkSecurityGroupName}
	re := regexp.MustCompile("/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft.Network/networkSecurityGroups/([^/]+)")
	groups := re.FindStringSubmatch(s.ControlPlane.Spec.Platform.NetworkSecurityGroupID)
	if len(groups) <= 1 {
		return ""
	}
	return groups[1]
}

// Name returns the cluster name for role assignment scope.
func (s *AROControlPlaneScope) Name() string {
	return s.ClusterName()
}

// RoleAssignmentSpecs returns the role assignment specifications for ARO HCP cluster.
func (s *AROControlPlaneScope) RoleAssignmentSpecs(_ *string) []azure.ResourceSpecGetter {
	// Get the HCP cluster spec
	ctx := context.Background()
	spec := s.HcpOpenShiftClusterSpecs(ctx)
	hcpOpenShiftClusterSpecs, ok := spec.(*hcpopenshiftclusters.HcpOpenShiftClustersSpec)
	if !ok {
		return []azure.ResourceSpecGetter{}
	}

	var specs []azure.ResourceSpecGetter

	// Generate all role assignment specifications based on the managed identities and required roles
	specs = append(specs, s.createRoleAssignmentSpecs(hcpOpenShiftClusterSpecs)...)

	return specs
}

// HasSystemAssignedIdentity returns false for ARO as it uses user-assigned identities.
func (s *AROControlPlaneScope) HasSystemAssignedIdentity() bool {
	return false
}

// RoleAssignmentResourceType returns the resource type for role assignments.
func (s *AROControlPlaneScope) RoleAssignmentResourceType() string {
	return azure.RoleAssignmentsList
}

// createRoleAssignmentSpecs creates all the role assignment specifications for ARO HCP cluster.
func (s *AROControlPlaneScope) createRoleAssignmentSpecs(spec *hcpopenshiftclusters.HcpOpenShiftClustersSpec) []azure.ResourceSpecGetter {
	var specs []azure.ResourceSpecGetter

	// We need an identities client to resolve principal IDs
	identitiesClient, err := identities.NewClient(s)
	if err != nil {
		return specs // Return empty specs if we can't create the client
	}

	s.addRoleAssignmentSpecs(spec, identitiesClient, &specs)
	return specs
}

// addRoleAssignmentSpecs adds all role assignment specifications to the provided specs slice.
func (s *AROControlPlaneScope) addRoleAssignmentSpecs(spec *hcpopenshiftclusters.HcpOpenShiftClustersSpec, identitiesClient identities.Client, specs *[]azure.ResourceSpecGetter) {
	// Helper function to create and add role assignment spec
	createSpec := func(principalResourceID string, roleDef roleDEF, scope, name string) {
		if principalResourceID == "" || scope == "" {
			return // Skip if invalid parameters
		}

		// Extract principal ID from managed identity resource ID
		principalID := s.extractPrincipalIDFromResourceID(principalResourceID, identitiesClient)
		if principalID == "" {
			return // Skip if we can't resolve the principal ID
		}

		roleSpec := &roleassignments.RoleAssignmentSpec{
			Name:             fmt.Sprintf("%s-%s", name, s.ClusterName()),
			MachineName:      s.ClusterName(),
			ResourceGroup:    s.ResourceGroup(),
			ResourceType:     azure.RoleAssignmentsList,
			PrincipalID:      &principalID,
			PrincipalType:    armauthorization.PrincipalTypeServicePrincipal, // User-assigned managed identities are service principals
			RoleDefinitionID: "/subscriptions/" + spec.SubscriptionID + "/providers/Microsoft.Authorization/roleDefinitions/" + string(roleDef),
			Scope:            scope,
		}
		*specs = append(*specs, roleSpec)
	}

	// ClusterAPI Azure managed identity has HCP Cluster API Provider role on subnet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.ClusterAPIAzureManagedIdentities, roleDEFhcpClusterAPIProvider, spec.SubnetID, "cluster-api-subnet")
	// Service managed identity has Reader role on ClusterAPI Azure managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.ClusterAPIAzureManagedIdentities, "service-reader-clusterapi")
	// Control Plane managed identity has HCP Control Plane Operator role on VNet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.ControlPlaneManagedIdentities, roleDEFhcpControlPlaneOperator, spec.VNetID, "controlplane-vnet")
	// Control Plane managed identity has HCP Control Plane Operator role on Network Security Group
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.ControlPlaneManagedIdentities, roleDEFhcpControlPlaneOperator, spec.NetworkSecurityGroupID, "controlplane-nsg")
	// Service managed identity has Reader role on Control Plane managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.ControlPlaneManagedIdentities, "service-reader-controlplane")
	// Cloud Controller Manager managed identity has Cloud Controller Manager role on subnet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.CloudControllerManagerManagedIdentities, roleDEFcloudControllerManager, spec.SubnetID, "ccm-subnet")
	// Cloud Controller Manager managed identity has Cloud Controller Manager role on Network Security Group
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.CloudControllerManagerManagedIdentities, roleDEFcloudControllerManager, spec.NetworkSecurityGroupID, "ccm-nsg")
	// Service managed identity has Reader role on Cloud Controller Manager managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.CloudControllerManagerManagedIdentities, "service-reader-ccm")
	// Ingress managed identity has Ingress Operator role on subnet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.IngressManagedIdentities, roleDEFingressOperator, spec.SubnetID, "ingress-subnet")
	// Service managed identity has Reader role on Ingress managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.IngressManagedIdentities, "service-reader-ingress")
	// Service managed identity has Reader role on Disk CSI Driver managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.DiskCsiDriverManagedIdentities, "service-reader-diskcsi")
	// File CSI Driver managed identity has File Storage Operator role on subnet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.FileCsiDriverManagedIdentities, roleDEFfileStorageOperator, spec.SubnetID, "filecsi-subnet")
	// File CSI Driver managed identity has File Storage Operator role on Network Security Group
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.FileCsiDriverManagedIdentities, roleDEFfileStorageOperator, spec.NetworkSecurityGroupID, "filecsi-nsg")
	// Service managed identity has Reader role on File CSI Driver managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.FileCsiDriverManagedIdentities, "service-reader-filecsi")
	// Service managed identity has Reader role on Image Registry managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.ImageRegistryManagedIdentities, "service-reader-imageregistry")
	// Cloud Network Config managed identity has Network Operator role on subnet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.CloudNetworkConfigManagedIdentities, roleDEFnetworkOperator, spec.SubnetID, "networkconfig-subnet")
	// Cloud Network Config managed identity has Network Operator role on VNet
	createSpec(spec.ManagedIdentities.ControlPlaneOperators.CloudNetworkConfigManagedIdentities, roleDEFnetworkOperator, spec.VNetID, "networkconfig-vnet")
	// Service managed identity has Reader role on Cloud Network Config managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFreader, spec.ManagedIdentities.ControlPlaneOperators.CloudNetworkConfigManagedIdentities, "service-reader-networkconfig")
	// Service managed identity has Federated Credentials role on Data Plane Disk CSI Driver managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFfederatedCredentials, spec.ManagedIdentities.DataPlaneOperators.DiskCsiDriverManagedIdentities, "service-fedcreds-dp-diskcsi")
	// Service managed identity has Federated Credentials role on Data Plane File CSI Driver managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFfederatedCredentials, spec.ManagedIdentities.DataPlaneOperators.FileCsiDriverManagedIdentities, "service-fedcreds-dp-filecsi")
	// Data Plane File CSI Driver managed identity has File Storage Operator role on subnet
	createSpec(spec.ManagedIdentities.DataPlaneOperators.FileCsiDriverManagedIdentities, roleDEFfileStorageOperator, spec.SubnetID, "dp-filecsi-subnet")
	// Data Plane File CSI Driver managed identity has File Storage Operator role on Network Security Group
	createSpec(spec.ManagedIdentities.DataPlaneOperators.FileCsiDriverManagedIdentities, roleDEFfileStorageOperator, spec.NetworkSecurityGroupID, "dp-filecsi-nsg")
	// Service managed identity has Federated Credentials role on Data Plane Image Registry managed identity
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFfederatedCredentials, spec.ManagedIdentities.DataPlaneOperators.ImageRegistryManagedIdentities, "service-fedcreds-dp-imageregistry")
	// Service managed identity has HCP Service Managed Identity role on VNet
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFhcpServiceManagedIdentity, spec.VNetID, "service-hcp-vnet")
	// Service managed identity has HCP Service Managed Identity role on subnet
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFhcpServiceManagedIdentity, spec.SubnetID, "service-hcp-subnet")
	// Service managed identity has HCP Service Managed Identity role on Network Security Group
	createSpec(spec.ManagedIdentities.ServiceManagedIdentity, roleDEFhcpServiceManagedIdentity, spec.NetworkSecurityGroupID, "service-hcp-nsg")
}

// extractPrincipalIDFromResourceID extracts the principal ID from a managed identity resource ID
// by getting the managed identity and reading its principal ID property.
func (s *AROControlPlaneScope) extractPrincipalIDFromResourceID(resourceID string, identitiesClient identities.Client) string {
	// Parse resource ID to extract resource name and resource group
	// Format: /subscriptions/{subscription}/resourceGroups/{rg}/providers/Microsoft.ManagedIdentity/userAssignedIdentities/{name}
	parts := strings.Split(resourceID, "/")
	if len(parts) < 9 {
		return ""
	}

	// Find the resource group and identity name
	var resourceGroup, identityName string
	for i, part := range parts {
		if part == "resourceGroups" && i+1 < len(parts) {
			resourceGroup = parts[i+1]
		}
		if part == "userAssignedIdentities" && i+1 < len(parts) {
			identityName = parts[i+1]
		}
	}

	if resourceGroup == "" || identityName == "" {
		return ""
	}

	// If no identities client is provided, we can't look up the principal ID
	if identitiesClient == nil {
		return ""
	}

	// Get the managed identity to retrieve its principal ID
	ctx := context.Background()
	identity, err := identitiesClient.Get(ctx, resourceGroup, identityName)
	if err != nil {
		return ""
	}

	if identity.Properties != nil && identity.Properties.PrincipalID != nil {
		return *identity.Properties.PrincipalID
	}

	return ""
}
