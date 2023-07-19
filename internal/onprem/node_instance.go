// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package onprem

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// NodeInstanceSchema manages Node instance level information of on prem cloud provider
func NodeInstanceSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Node instances associated with the provider.",
		Optional:    true,
		Type:        schema.TypeList,
		MinItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"instance_name": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Node instance name provided by the user.",
				},
				"instance_type": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Node instance type.",
				},
				"instance_type_code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Node instance type code.",
				},
				"ip": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "IP address of node.",
				},
				"node_name": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "Node name allocated during universe creation.",
				},
				"node_uuid": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "Node UUID.",
				},
				"node_configs": {
					Type:        schema.TypeList,
					Optional:    true,
					Description: "Node Configurations.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"type": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "Type.",
							},
							"value": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "Value.",
							},
						},
					},
				},
				"region": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Region of node.",
				},
				"zone": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Zone of node.",
				},
				"zone_uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Zone UUID of node.",
				},
				"ssh_user": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "SSH user.",
				},
				"details_json": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Details of the node",
				},
				"in_use": {
					Type:        schema.TypeBool,
					Optional:    true,
					Computed:    true,
					Description: "Is the node used in a universe.",
				},
			},
		},
	}
}

func nodeInstancesCreate(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	nodeInstanceList []interface{}) ([]map[string]interface{}, error) {
	nIL := make([]client.NodeInstance, 0)
	if len(nodeInstanceList) == 0 {
		return nil, fmt.Errorf("Node Instance List is empty")
	}
	// this is to ensure that the order of node instances in the state
	// file is the same as the one in the config file to avoid difference
	// during subsequent terraform apply commands
	nodeInstanceIPList := make([]string, 0)
	for _, m := range nodeInstanceList {
		n := m.(map[string]interface{})
		nodeInstanceIPList = append(nodeInstanceIPList, n["ip"].(string))
	}
	regionToReq := buildNodeInstanceFormData(nodeInstanceList)
	for rName, zoneToReq := range regionToReq {
		// fetch region uuid from region name
		rUUID, err := fetchRegionUUIDFromRegionName(ctx, c, cUUID, pUUID, rName)
		if err != nil {
			return nil, err
		}
		for az, req := range zoneToReq {
			// fetch zone uuid from zone name
			azUUID, err := fetchZoneUUIDFromZoneName(ctx, c, cUUID, pUUID, rUUID, az)
			if err != nil {
				return nil, err
			}
			r, response, err := c.NodeInstancesApi.CreateNodeInstance(
				ctx, cUUID, azUUID).NodeInstance(req).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
					"Onprem Node Instances", "Create")
				return nil, errMessage
			}
			for _, n := range r {
				nIL = append(nIL, n)
			}
		}
	}
	nodeInstanceRespList := flattenNodeInstances(nIL, nodeInstanceIPList)
	return nodeInstanceRespList, nil
}

func nodeInstancesRead(ctx context.Context, c *client.APIClient, cUUID, pUUID string) (
	[]client.NodeInstance, error) {
	r, response, err := c.NodeInstancesApi.ListByProvider(ctx, cUUID, pUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Onprem Node Instances", "Read")
		return nil, errMessage
	}
	return r, nil

}

func nodeInstanceDelete(ctx context.Context, c *client.APIClient, cUUID, pUUID,
	nodeIP string) error {
	_, response, err := c.NodeInstancesApi.DeleteInstance(ctx, cUUID, pUUID,
		nodeIP).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Onprem Node Instances", "Delete")
		return errMessage
	}
	return nil
}

func buildNodeInstanceFormData(ni interface{}) map[string](map[string]client.NodeInstanceFormData) {
	nodeInstances := ni.([]interface{})
	nodes := make(map[string](map[string]([]client.NodeInstanceData)))
	regionToNodesForm := make(map[string](map[string]client.NodeInstanceFormData))

	for _, n := range nodeInstances {
		nodeInstance := n.(map[string]interface{})
		node := client.NodeInstanceData{
			InstanceName: nodeInstance["instance_name"].(string),
			InstanceType: nodeInstance["instance_type"].(string),
			Ip:           nodeInstance["ip"].(string),
			NodeConfigs:  buildNodeConfig(nodeInstance["node_config"]),
			NodeName:     utils.GetStringPointer(nodeInstance["node_name"].(string)),
			Region:       nodeInstance["region"].(string),
			Zone:         nodeInstance["zone"].(string),
			SshUser:      nodeInstance["ssh_user"].(string),
		}
		regionName := node.GetRegion()
		if regionName != "" && len(nodes[regionName]) == 0 {
			nodes[regionName] = make(map[string]([]client.NodeInstanceData))
		}

		zoneName := node.GetZone()
		if zoneName != "" && len(nodes[regionName][zoneName]) == 0 {
			nodes[regionName][zoneName] = make([]client.NodeInstanceData, 0)
		}
		nodes[regionName][zoneName] = append(nodes[regionName][zoneName], node)
	}
	for region, regionBasedNodes := range nodes {
		zoneToNodesForm := make(map[string]client.NodeInstanceFormData)
		for az, n := range regionBasedNodes {
			formData := client.NodeInstanceFormData{
				Nodes: n,
			}
			zoneToNodesForm[az] = formData
		}
		regionToNodesForm[region] = zoneToNodesForm
	}
	return regionToNodesForm
}

func buildNodeConfig(nList interface{}) *[]client.NodeConfig {
	if nList == nil {
		return nil
	}
	list := nList.([]interface{})
	nodeConfigList := make([]client.NodeConfig, 0)
	for _, v := range list {
		config := v.(map[string]interface{})
		vD := client.NodeConfig{
			Type:  config["type"].(string),
			Value: config["value"].(string),
		}
		nodeConfigList = append(nodeConfigList, vD)
	}
	return &nodeConfigList
}

func flattenNodeInstances(nodeInstanceList []client.NodeInstance, order []string) (
	res []map[string]interface{}) {
	orderLength := len(order)
	res = make([]map[string]interface{}, orderLength)
	for _, n := range nodeInstanceList {
		details := n.GetDetails()
		i := map[string]interface{}{
			"instance_name":      n.GetInstanceName(),
			"instance_type":      details.GetInstanceType(),
			"instance_type_code": n.GetInstanceTypeCode(),
			"ip":                 details.GetIp(),
			"node_name":          n.GetNodeName(),
			"node_uuid":          n.GetNodeUuid(),
			"node_configs":       flattenNodeConfig(details.GetNodeConfigs()),
			"region":             details.GetRegion(),
			"zone":               details.GetZone(),
			"zone_uuid":          n.GetZoneUuid(),
			"ssh_user":           details.GetSshUser(),
			"details_json":       n.GetDetailsJson(),
			"in_use":             n.GetInUse(),
		}
		index := slices.Index(order, details.GetIp())
		if index != -1 {
			res[index] = i
		} else {
			res = append(res, i)
		}
	}
	return res
}

func flattenNodeConfig(nodeInstanceConfig []client.NodeConfig) (res []map[string]interface{}) {
	for _, nC := range nodeInstanceConfig {
		i := map[string]interface{}{
			"type":  nC.GetType(),
			"value": nC.GetValue(),
		}
		res = append(res, i)
	}
	return res
}
