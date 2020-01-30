// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package infrastructure

import (
	"path/filepath"

	api "github.com/gardener/gardener-extension-provider-openstack/pkg/apis/openstack"
	"github.com/gardener/gardener-extension-provider-openstack/pkg/apis/openstack/helper"
	apiv1alpha1 "github.com/gardener/gardener-extension-provider-openstack/pkg/apis/openstack/v1alpha1"
	"github.com/gardener/gardener-extension-provider-openstack/pkg/internal"
	"github.com/gardener/gardener-extensions/pkg/controller"
	"github.com/gardener/gardener-extensions/pkg/terraformer"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TerraformerPurpose is a constant for the complete Terraform setup with purpose 'infrastructure'.
	TerraformerPurpose = "infra"
	// TerraformOutputKeySSHKeyName key for accessing SSH key name from outputs in terraform
	TerraformOutputKeySSHKeyName = "key_name"
	// TerraformOutputKeyRouterID is the id the router between provider network and the worker subnet.
	TerraformOutputKeyRouterID = "router_id"
	// TerraformOutputKeyNetworkID is the private worker network.
	TerraformOutputKeyNetworkID = "network_id"
	// TerraformOutputKeySecurityGroupID is the id of worker security group.
	TerraformOutputKeySecurityGroupID = "security_group_id"
	// TerraformOutputKeySecurityGroupName is the name of the worker security group.
	TerraformOutputKeySecurityGroupName = "security_group_name"
	// TerraformOutputKeyFloatingNetworkID is the id of the provider network.
	TerraformOutputKeyFloatingNetworkID = "floating_network_id"
	// TerraformOutputKeySubnetID is the id of the worker subnet.
	TerraformOutputKeySubnetID = "subnet_id"
	// DefaultRouterID is the computed router ID as generated by terraform.
	DefaultRouterID = "${openstack_networking_router_v2.router.id}"
)

var (
	// ChartsPath is the path to the charts
	ChartsPath = filepath.Join("controllers", "provider-openstack", "charts")
	// InternalChartsPath is the path to the internal charts
	InternalChartsPath = filepath.Join(ChartsPath, "internal")
	// StatusTypeMeta is the TypeMeta of the GCP InfrastructureStatus
	StatusTypeMeta = metav1.TypeMeta{
		APIVersion: apiv1alpha1.SchemeGroupVersion.String(),
		Kind:       "InfrastructureStatus",
	}
)

// ComputeTerraformerChartValues computes the values for the OpenStack Terraformer chart.
func ComputeTerraformerChartValues(
	infra *extensionsv1alpha1.Infrastructure,
	credentials *internal.Credentials,
	config *api.InfrastructureConfig,
	cluster *controller.Cluster,
) (map[string]interface{}, error) {
	var (
		createRouter = true
		routerID     = DefaultRouterID
	)

	if router := config.Networks.Router; router != nil {
		createRouter = false
		routerID = router.ID
	}

	cloudProfileConfig, err := helper.CloudProfileConfigFromCluster(cluster)
	if err != nil {
		return nil, err
	}

	keyStoneURL, err := helper.FindKeyStoneURL(cloudProfileConfig.KeyStoneURLs, cloudProfileConfig.KeyStoneURL, infra.Spec.Region)
	if err != nil {
		return nil, err
	}

	workersCIDR := config.Networks.Workers
	// Backwards compatibility - remove this code in a future version.
	if workersCIDR == "" {
		workersCIDR = config.Networks.Worker
	}

	return map[string]interface{}{
		"openstack": map[string]interface{}{
			"authURL":          keyStoneURL,
			"domainName":       credentials.DomainName,
			"tenantName":       credentials.TenantName,
			"region":           infra.Spec.Region,
			"floatingPoolName": config.FloatingPoolName,
		},
		"create": map[string]interface{}{
			"router": createRouter,
		},
		"dnsServers":   cloudProfileConfig.DNSServers,
		"sshPublicKey": string(infra.Spec.SSHPublicKey),
		"router": map[string]interface{}{
			"id": routerID,
		},
		"clusterName": infra.Namespace,
		"networks": map[string]interface{}{
			"workers": workersCIDR,
		},
		"outputKeys": map[string]interface{}{
			"routerID":          TerraformOutputKeyRouterID,
			"networkID":         TerraformOutputKeyNetworkID,
			"keyName":           TerraformOutputKeySSHKeyName,
			"securityGroupID":   TerraformOutputKeySecurityGroupID,
			"securityGroupName": TerraformOutputKeySecurityGroupName,
			"floatingNetworkID": TerraformOutputKeyFloatingNetworkID,
			"subnetID":          TerraformOutputKeySubnetID,
		},
	}, nil
}

// RenderTerraformerChart renders the openstack-infra chart with the given values.
func RenderTerraformerChart(
	renderer chartrenderer.Interface,
	infra *extensionsv1alpha1.Infrastructure,
	credentials *internal.Credentials,
	config *api.InfrastructureConfig,
	cluster *controller.Cluster,
) (*TerraformFiles, error) {
	values, err := ComputeTerraformerChartValues(infra, credentials, config, cluster)
	if err != nil {
		return nil, err
	}

	release, err := renderer.Render(filepath.Join(InternalChartsPath, "openstack-infra"), "openstack-infra", infra.Namespace, values)
	if err != nil {
		return nil, err
	}

	return &TerraformFiles{
		Main:      release.FileContent("main.tf"),
		Variables: release.FileContent("variables.tf"),
		TFVars:    []byte(release.FileContent("terraform.tfvars")),
	}, nil
}

// TerraformFiles are the files that have been rendered from the infrastructure chart.
type TerraformFiles struct {
	Main      string
	Variables string
	TFVars    []byte
}

// TerraformState is the Terraform state for an infrastructure.
type TerraformState struct {
	// SSHKeyName key for accessing SSH key name from outputs in terraform
	SSHKeyName string
	// RouterID is the id the router between provider network and the worker subnet.
	RouterID string
	// NetworkID is the private worker network.
	NetworkID string
	// SubnetID is the id of the worker subnet.
	SubnetID string
	// FloatingNetworkID is the id of the provider network.
	FloatingNetworkID string
	// SecurityGroupID is the id of worker security group.
	SecurityGroupID string
	// SecurityGroupName is the name of the worker security group.
	SecurityGroupName string
}

// ExtractTerraformState extracts the TerraformState from the given Terraformer.
func ExtractTerraformState(tf terraformer.Terraformer, config *api.InfrastructureConfig) (*TerraformState, error) {
	outputKeys := []string{
		TerraformOutputKeySSHKeyName,
		TerraformOutputKeyRouterID,
		TerraformOutputKeyNetworkID,
		TerraformOutputKeySubnetID,
		TerraformOutputKeyFloatingNetworkID,
		TerraformOutputKeySecurityGroupID,
		TerraformOutputKeySecurityGroupName,
	}

	vars, err := tf.GetStateOutputVariables(outputKeys...)
	if err != nil {
		return nil, err
	}

	state := &TerraformState{
		SSHKeyName:        vars[TerraformOutputKeySSHKeyName],
		RouterID:          vars[TerraformOutputKeyRouterID],
		NetworkID:         vars[TerraformOutputKeyNetworkID],
		SubnetID:          vars[TerraformOutputKeySubnetID],
		FloatingNetworkID: vars[TerraformOutputKeyFloatingNetworkID],
		SecurityGroupID:   vars[TerraformOutputKeySecurityGroupID],
		SecurityGroupName: vars[TerraformOutputKeySecurityGroupName],
	}
	return state, nil
}

// StatusFromTerraformState computes an InfrastructureStatus from the given
// Terraform variables.
func StatusFromTerraformState(state *TerraformState) *apiv1alpha1.InfrastructureStatus {
	var (
		status = &apiv1alpha1.InfrastructureStatus{
			TypeMeta: metav1.TypeMeta{
				APIVersion: apiv1alpha1.SchemeGroupVersion.String(),
				Kind:       "InfrastructureStatus",
			},
			Networks: apiv1alpha1.NetworkStatus{
				ID: state.NetworkID,
				FloatingPool: apiv1alpha1.FloatingPoolStatus{
					ID: state.FloatingNetworkID,
				},
				Router: apiv1alpha1.RouterStatus{
					ID: state.RouterID,
				},
				Subnets: []apiv1alpha1.Subnet{
					{
						Purpose: apiv1alpha1.PurposeNodes,
						ID:      state.SubnetID,
					},
				},
			},
			SecurityGroups: []apiv1alpha1.SecurityGroup{
				{
					Purpose: apiv1alpha1.PurposeNodes,
					ID:      state.SecurityGroupID,
					Name:    state.SecurityGroupName,
				},
			},
			Node: apiv1alpha1.NodeStatus{
				KeyName: state.SSHKeyName,
			},
		}
	)

	return status
}

// ComputeStatus computes the status based on the Terraformer and the given InfrastructureConfig.
func ComputeStatus(tf terraformer.Terraformer, config *api.InfrastructureConfig) (*apiv1alpha1.InfrastructureStatus, error) {
	state, err := ExtractTerraformState(tf, config)
	if err != nil {
		return nil, err
	}

	status := StatusFromTerraformState(state)
	status.Networks.FloatingPool.Name = config.FloatingPoolName
	return status, nil
}
