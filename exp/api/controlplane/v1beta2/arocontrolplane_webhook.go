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
	"net"
	"reflect"
	"regexp"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	webhookutils "sigs.k8s.io/cluster-api-provider-azure/util/webhook"
)

var (
	ocpSemver                  = regexp.MustCompile(`^openshift-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([-0-9a-zA-Z_\.+]*)?$`)
	kubeSemver                 = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([-0-9a-zA-Z_\.+]*)?$`)
	rMaxNodeProvisionTime      = regexp.MustCompile(`^(\d+)m$`)
	rScaleDownTime             = regexp.MustCompile(`^(\d+)m$`)
	rScaleDownDelayAfterDelete = regexp.MustCompile(`^(\d+)s$`)
	rScanInterval              = regexp.MustCompile(`^(\d+)s$`)
)

// SetupAROControlPlaneWebhookWithManager sets up and registers the webhook with the manager.
func SetupAROControlPlaneWebhookWithManager(mgr ctrl.Manager) error {
	mw := &aroControlPlaneWebhook{Client: mgr.GetClient()}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&AROControlPlane{}).
		WithDefaulter(mw).
		WithValidator(mw).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta2-arocontrolplane,mutating=true,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=arocontrolplanes,verbs=create;update,versions=v1beta2,name=default.arocontrolplanes.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta2

// aroControlPlaneWebhook implements a validating and defaulting webhook for AROControlPlane.
type aroControlPlaneWebhook struct {
	Client client.Client
}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (mw *aroControlPlaneWebhook) Default(_ context.Context, obj runtime.Object) error {
	m, ok := obj.(*AROControlPlane)
	if !ok {
		return apierrors.NewBadRequest("expected an AROControlPlane")
	}

	m.Spec.Version = setDefaultOCPVersion(m.Spec.Version)

	/*

		if err := m.setDefaultSSHPublicKey(); err != nil {
			ctrl.Log.WithName("AROControlPlaneWebHookLogger").Error(err, "setDefaultSSHPublicKey failed")
		}

		m.setDefaultResourceGroupName()
		m.setDefaultNodeResourceGroupName()
		m.setDefaultVirtualNetwork()
		m.setDefaultSubnet()
		m.setDefaultOIDCIssuerProfile()
		m.setDefaultDNSPrefix()
		m.setDefaultAKSExtensions()
	*/
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta2-arocontrolplane,mutating=false,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=arocontrolplanes,versions=v1beta2,name=validation.arocontrolplanes.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta2

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (mw *aroControlPlaneWebhook) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*AROControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest("expected an AROControlPlane")
	}

	return nil, m.Validate(mw.Client)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (mw *aroControlPlaneWebhook) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	old, ok := oldObj.(*AROControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest("expected an AROControlPlane")
	}
	m, ok := newObj.(*AROControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest("expected an AROControlPlane")
	}

	immutableFields := []struct {
		path *field.Path
		old  interface{}
		new  interface{}
	}{
		{field.NewPath("spec", "platform", "resourceGroup"), old.Spec.Platform.ResourceGroup, m.Spec.Platform.ResourceGroup},
		{field.NewPath("spec", "platform", "location"), old.Spec.Platform.Location, m.Spec.Platform.Location},
		{field.NewPath("spec", "platform", "networkSecurityGroupID"), old.Spec.Platform.NetworkSecurityGroupID, m.Spec.Platform.NetworkSecurityGroupID},
	}

	for _, f := range immutableFields {
		if err := webhookutils.ValidateImmutable(f.path, f.old, f.new); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	if m.Spec.DomainPrefix != "" {
		if err := webhookutils.ValidateImmutable(
			field.NewPath("spec", "domainPrefix"),
			old.Spec.DomainPrefix,
			m.Spec.DomainPrefix,
		); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	// Consider removing this once moves out of preview
	// Updating outboundType after cluster creation (PREVIEW)
	// https://learn.microsoft.com/en-us/azure/aks/egress-outboundtype#updating-outboundtype-after-cluster-creation-preview
	if err := webhookutils.ValidateImmutable(
		field.NewPath("spec", "platform", "outboundType"),
		old.Spec.Platform.OutboundType,
		m.Spec.Platform.OutboundType); err != nil {
		allErrs = append(allErrs, err)
	}

	if errs := m.validateNetworkUpdate(old); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	if len(allErrs) == 0 {
		return nil, m.Validate(mw.Client)
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind(AROControlPlaneKind).GroupKind(), m.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (mw *aroControlPlaneWebhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// Validate the Azure Managed Control Plane and return an aggregate error.
func (m *AROControlPlane) Validate(cli client.Client) error {
	var allErrs field.ErrorList
	validators := []func(client client.Client) field.ErrorList{
		m.validateIdentity,
		m.validateDNSPrefix,
	}
	for _, validator := range validators {
		if err := validator(cli); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	allErrs = append(allErrs, validateOCPVersion(
		m.Spec.Version,
		field.NewPath("spec").Child("version"))...)

	allErrs = append(allErrs, validateNetwork(m.Spec.Network, field.NewPath("spec"))...)

	allErrs = append(allErrs, validateName(m.Name, field.NewPath("name"))...)

	/*
		allErrs = append(allErrs, validateAutoScalerProfile(m.Spec.AutoScalerProfile, field.NewPath("spec").Child("autoScalerProfile"))...)

		allErrs = append(allErrs, validateAKSExtensions(m.Spec.Extensions, field.NewPath("spec").Child("aksExtensions"))...)

		allErrs = append(allErrs, m.Spec.AROControlPlaneClassSpec.validateSecurityProfile()...)

		allErrs = append(allErrs, validateNetworkPolicy(m.Spec.NetworkPolicy, m.Spec.NetworkDataplane, field.NewPath("spec").Child("networkPolicy"))...)

		allErrs = append(allErrs, validateNetworkDataplane(m.Spec.NetworkDataplane, m.Spec.NetworkPolicy, m.Spec.NetworkPluginMode, field.NewPath("spec").Child("networkDataplane"))...)

		allErrs = append(allErrs, validateAPIServerAccessProfile(m.Spec.APIServerAccessProfile, field.NewPath("spec").Child("apiServerAccessProfile"))...)

		allErrs = append(allErrs, validateAMCPVirtualNetwork(m.Spec.VirtualNetwork, field.NewPath("spec").Child("virtualNetwork"))...)

		allErrs = append(allErrs, validateFleetsMember(m.Spec.FleetsMember, field.NewPath("spec").Child("fleetsMember"))...)
	*/

	return allErrs.ToAggregate()
}

func (m *AROControlPlane) validateDNSPrefix(_ client.Client) field.ErrorList {
	if m.Spec.DomainPrefix == "" {
		return nil
	}

	// Regex pattern for DNS prefix validation
	// 1. Between 1 and 54 characters long: {1,54}
	// 2. Alphanumerics and hyphens: [a-zA-Z0-9-]
	// 3. Start and end with alphanumeric: ^[a-zA-Z0-9].*[a-zA-Z0-9]$
	pattern := `^[a-zA-Z0-9][a-zA-Z0-9-]{0,52}[a-zA-Z0-9]$`
	regex := regexp.MustCompile(pattern)
	if regex.MatchString(m.Spec.DomainPrefix) {
		return nil
	}
	allErrs := field.ErrorList{
		field.Invalid(field.NewPath("spec", "domainPrefix"), m.Spec.DomainPrefix, "DomainPrefix is invalid, does not match regex: "+pattern),
	}
	return allErrs
}

// validateOCPVersion validates the Kubernetes version.
func validateOCPVersion(version string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if !ocpSemver.MatchString(version) {
		allErrs = append(allErrs, field.Invalid(fldPath, version, "must be a openshift-<valid semantic version>"))
	}

	return allErrs
}

func validateNetwork(virtualNetwork *NetworkSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !reflect.DeepEqual(virtualNetwork, NetworkSpec{}) {
		_, _, vnetErr := net.ParseCIDR(virtualNetwork.MachineCIDR)
		if vnetErr != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("MachineCIDR"), virtualNetwork.MachineCIDR, "CIDR block is invalid"))
		}
		_, _, vnetErr = net.ParseCIDR(virtualNetwork.ServiceCIDR)
		if vnetErr != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("ServiceCIDR"), virtualNetwork.ServiceCIDR, "CIDR block is invalid"))
		}
		_, _, vnetErr = net.ParseCIDR(virtualNetwork.PodCIDR)
		if vnetErr != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("PodCIDR"), virtualNetwork.PodCIDR, "CIDR block is invalid"))
		}
		/*
			if vnetErr == nil && subnetErr == nil && !parentNet.Contains(subnetIP) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("CIDRBlock"), virtualNetwork.CIDRBlock, "pre-existing virtual networks CIDR block should contain the subnet CIDR block"))
			}
		*/
	}
	return allErrs
}

// validateNetworkUpdate validates update to VirtualNetwork.
func (m *AROControlPlane) validateNetworkUpdate(old *AROControlPlane) field.ErrorList {
	var allErrs field.ErrorList

	if old.Spec.Network.MachineCIDR != m.Spec.Network.MachineCIDR {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "network", "machineCIDR"),
				m.Spec.Network.MachineCIDR,
				"Network CIDR is immutable"))
	}

	if old.Spec.Network.PodCIDR != m.Spec.Network.PodCIDR {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "network", "podCIDR"),
				m.Spec.Network.PodCIDR,
				"Network CIDR is immutable"))
	}

	if old.Spec.Network.ServiceCIDR != m.Spec.Network.ServiceCIDR {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "network", "serviceCIDR"),
				m.Spec.Network.ServiceCIDR,
				"Network CIDR is immutable"))
	}

	if old.Spec.Network.NetworkType != m.Spec.Network.NetworkType {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "network", "networkType"),
				m.Spec.Network.NetworkType,
				"Network type is immutable"))
	}

	if old.Spec.Network.HostPrefix != m.Spec.Network.HostPrefix {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "network", "hostPrefix"),
				m.Spec.Network.HostPrefix,
				"Network host prefix is immutable"))
	}

	if old.Spec.Platform.Subnet != m.Spec.Platform.Subnet {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "platform", "subnet"),
				m.Spec.Platform.Subnet,
				"Subnet id is immutable"))
	}

	return allErrs
}

func validateName(name string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if lName := strings.ToLower(name); strings.Contains(lName, "microsoft") ||
		strings.Contains(lName, "windows") {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("Name"), name,
			"cluster name is invalid because 'MICROSOFT' and 'WINDOWS' can't be used as either a whole word or a substring in the name"))
	}

	return allErrs
}

// validateIdentity validates an Identity.
func (m *AROControlPlane) validateIdentity(_ client.Client) field.ErrorList {
	var allErrs field.ErrorList

	if m.Spec.IdentityRef != nil {
		if m.Spec.IdentityRef.Name == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "identityRef", "name"), m.Spec.IdentityRef.Name, "cannot be empty"))
		}
	}

	if len(allErrs) > 0 {
		return allErrs
	}

	return nil
}

func setDefaultOCPVersion(version string) string {
	if version != "" && !strings.HasPrefix(version, "openshift-v") && strings.HasPrefix(version, "v") {
		normalizedVersion := "openshift-" + version
		version = normalizedVersion
	}
	if version != "" && !strings.HasPrefix(version, "openshift-v") {
		normalizedVersion := "openshift-v" + version
		version = normalizedVersion
	}
	return version
}
