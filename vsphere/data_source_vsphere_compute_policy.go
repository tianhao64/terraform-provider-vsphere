package vsphere

import (
	"fmt"

	"github.com/hashicorp/terraform/helper/schema"

	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vcenter/compute"
)

func dataSourceVSphereComputePolicy() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceVSphereComputePolicyRead,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the compute policy.",
			},
			"description": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Description of the compute policy.",
			},
			"policy_type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Type of the compute policy.",
			},
			"vm_tag": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The unique identifier of the vm tag.",
			},
			"host_tag": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The unique identifier of the host tag for VM-Host affinity/anti affinity rules",
			},
		},
	}
}

func dataSourceVSphereComputePolicyRead(d *schema.ResourceData, meta interface{}) error {
	policyName := d.Get("name").(string)
	connector := meta.(*VSphereClient).vApiConnector
	policyClient := compute.NewDefaultPoliciesClient(connector)
	policySummaries, err := policyClient.List()
	if err != nil {
		return err
	}

	for _, summary := range policySummaries {
		if summary.Name == policyName {
			d.SetId(summary.Policy)
			d.Set("name", summary.Name)
			d.Set("policy_type", capabilityToPolicyType(summary.Capability))
			return nil
		}
	}

	return fmt.Errorf("error fetching compute policy: %s", err)
}
