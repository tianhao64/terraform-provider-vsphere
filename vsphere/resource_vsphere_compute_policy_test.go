package vsphere

import (
	"fmt"
	// "os"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"

	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vapi/std/errors"
	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vcenter/compute"
	// "gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/runtime/data"
)

const testAccCheckVSphereComputePolicyResourceName = "vsphere_compute_policy.testPolicy"

const testAccCheckVSphereComputePolicyConfig = `
data "vsphere_tag_category" "category" {
	name = "testCategory"
}

data "vsphere_tag" "tag" {
	name = "testTag"
	category_id = "${data.vsphere_tag_category.category.id}"
}

resource "vsphere_compute_policy" "testPolicy" {
	name = "testPolicy"
	description = "vm_host_affinit"
	vm_tag = "${data.vsphere_tag.tag.id}"
	host_tag = "${data.vsphere_tag.tag.id}"
	policy_type = "vm_host_affinity"
}
`

func TestAccResourceVSphereComputePolicy_basic(t *testing.T) {

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckVSphereComputePolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckVSphereComputePolicyConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVSphereComputePolicyExists(testAccCheckVSphereComputePolicyResourceName),
					resource.TestCheckResourceAttr("vsphere_compute_policy.testPolicy", "name", "testPolicy"),
					resource.TestCheckResourceAttr("vsphere_compute_policy.testPolicy", "description", "vm_host_affinit"),
					resource.TestCheckResourceAttr("vsphere_compute_policy.testPolicy", "policy_type", "vm_host_affinity"),
				),
			},
		},
	})
}

func testAccCheckVSphereComputePolicyDestroy(s *terraform.State) error {
	connector := testAccProvider.Meta().(*VSphereClient).vApiConnector
	policyClient := compute.NewDefaultPoliciesClient(connector)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vsphere_compute_policy" {
			continue
		}

		if _, err := policyClient.Get(rs.Primary.ID); err != nil {
			if err.Error() == (errors.NotFound{}.Error()) {
				return nil
			}
			return err
		} else {
			return fmt.Errorf("compute policy '%s' still exists", rs.Primary.Attributes["name"])
		}
	}
	return nil
}

func testAccCheckVSphereComputePolicyExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("resource not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		connector := testAccProvider.Meta().(*VSphereClient).vApiConnector
		policyClient := compute.NewDefaultPoliciesClient(connector)
		_, err := policyClient.Get(rs.Primary.ID)

		if err != nil {
			if err.Error() == (errors.NotFound{}.Error()) {
				return fmt.Errorf("compute policy does not exist: %s", err.Error())
			}
			return err
		}
		return nil
	}
}
