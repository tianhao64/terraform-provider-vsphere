package vsphere

import (
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/structure"

	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vapi/std/errors"
	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vcenter/compute"
	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/runtime/data"
)

const resourceVSphereComputePolicyName = "vsphere_compute_policy"

const (
	computePolicyTypeVmHostAffinity        = "vm_host_affinity"
	computePolicyTypeVmHostAntiAffinity    = "vm_host_anti_affinity"
	computePolicyTypeVmVmAffinity          = "vm_vm_affinity"
	computePolicyTypeVmVmAntiAffinity      = "vm_vm_anti_affinity"
)

var computePolicyTypeAllowedValues = []string{
	computePolicyTypeVmHostAffinity,
	computePolicyTypeVmHostAntiAffinity,
	computePolicyTypeVmVmAffinity,
	computePolicyTypeVmVmAntiAffinity,
}

func resourceVSphereComputePolicy() *schema.Resource {
	return &schema.Resource{
		Create: resourceVSphereComputePolicyCreate,
		Read:   resourceVSphereComputePolicyRead,
		Delete: resourceVSphereComputePolicyDelete,
		Importer: &schema.ResourceImporter{
			State: resourceVSphereComputePolicyImport,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the compute policy.",
			},
			"description": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Description of the compute policy.",
			},
			"policy_type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "Type of the compute policy.",
				ValidateFunc: validation.StringInSlice(computePolicyTypeAllowedValues, false),
			},
			"vm_tag": {
				Type:        schema.TypeString,
				Description: "The unique identifier of the vm tag.",
				Required:    true,
				ForceNew:    true,
			},
			"host_tag": {
				Type:        schema.TypeString,
				Description: "The unique identifier of the host tag for VM-Host affinity/anti affinity rules",
				Optional:    true,
				ForceNew:    true,
			},
		},
	}
}

func resourceVSphereComputePolicyCreate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning create", resourceVSphereComputePolicyIDString(d))

	fields := make(map[string]data.DataValue)
	fields["name"] = data.NewStringValue(d.Get("name").(string))
	fields["description"] = data.NewStringValue(d.Get("description").(string))
	fields["vm_tag"] = data.NewStringValue(d.Get("vm_tag").(string))
	fields["host_tag"] = data.NewStringValue(d.Get("host_tag").(string))
	capabilityFullName := "com.vmware.vcenter.compute.policies.capabilities." + d.Get("policy_type").(string)
	fields["capability"] = data.NewStringValue(capabilityFullName)
	var createSpec = data.NewStructValue("", fields)

	connector := meta.(*VSphereClient).vApiConnector
	policyClient := compute.NewDefaultPoliciesClient(connector)
	result, err := policyClient.Create(createSpec)
	if err != nil {
		return err
	}

	// All done!
	d.SetId(result)
	log.Printf("[DEBUG] %s: Create finished successfully", resourceVSphereComputePolicyIDString(d))
	return resourceVSphereComputePolicyRead(d, meta)
}

func resourceVSphereComputePolicyRead(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning read", resourceVSphereComputePolicyIDString(d))

	connector := meta.(*VSphereClient).vApiConnector
	policyClient := compute.NewDefaultPoliciesClient(connector)
	summaryStruct, err := policyClient.Get(d.Id())
	if err != nil {
		if err.Error() == (errors.NotFound{}.Error()) {
			d.SetId("")
			return nil
		}
		return err
	}

	name, err := summaryStruct.String("name")
	if err != nil {
		return err
	}

	description, err := summaryStruct.String("description")
	if err != nil {
		return err
	}

	if err = d.Set("name", name); err != nil {
		return err
	}
	if err = d.Set("description", description); err != nil {
		return err
	}

	log.Printf("[DEBUG] %s: Read completed successfully", d.Id())
	return nil
}

func resourceVSphereComputePolicyDelete(d *schema.ResourceData, meta interface{}) error {
	connector := meta.(*VSphereClient).vApiConnector
	policyClient := compute.NewDefaultPoliciesClient(connector)
	return policyClient.Delete(d.Id())
}

func resourceVSphereComputePolicyImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// TODO
	return []*schema.ResourceData{d}, nil
}

// resourceVSphereComputePolicyIDString prints a friendly string for the
// vsphere_compute_policy resource.
func resourceVSphereComputePolicyIDString(d structure.ResourceIDStringer) string {
	return structure.ResourceIDString(d, resourceVSphereComputePolicyName)
}
