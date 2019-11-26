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
		},
	}
}

func dataSourceVSphereComputePolicyRead(d *schema.ResourceData, meta interface{}) error {
	connector := meta.(*VSphereClient).vApiConnector
	policyClient := compute.NewDefaultPoliciesClient(connector)
	policyName := d.Get("name").(string)

	summaries, err := policyClient.List()
	if err != nil {
		return fmt.Errorf("error fetching compute policy: %s", err)
	}

	for _, summary := range summaries {
		if summary.Name == policyName {
			d.SetId(summary.Policy)
			return nil
		}
	}
	return nil
}
