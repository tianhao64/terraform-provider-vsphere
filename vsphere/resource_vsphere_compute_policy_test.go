package vsphere

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"

	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vapi/std/errors"
	"gitlab.eng.vmware.com/golangsdk/vsphere-automation-sdk-go/vapi/bindings/vcenter/compute"
)

const testAccCheckVSphereComputePolicyResourceName = "vsphere_compute_policy.terraform_test_policy"

const testAccCheckVSphereComputePolicyConfig = `
resource "vsphere_tag_category" "terraform_test_category" {
	name        = "terraform-test-tag-category"
	description = "description"
	cardinality = "MULTIPLE"

	associable_types = [
	  "HostSystem",
	  "VirtualMachine"
	]
}

resource "vsphere_tag" "terraform_test_tag" {
	name        = "terraform-test-tag"
	description = "description"
	category_id = "${vsphere_tag_category.terraform_test_category.id}"
}

resource "vsphere_compute_policy" "terraform_test_policy" {
	name = "testPolicy"
	description = "vm_host_affinity"
	vm_tag = "${vsphere_tag.terraform_test_tag.id}"
	host_tag = "${vsphere_tag.terraform_test_tag.id}"
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
					resource.TestCheckResourceAttr(testAccCheckVSphereComputePolicyResourceName, "name", "testPolicy"),
					resource.TestCheckResourceAttr(testAccCheckVSphereComputePolicyResourceName, "description", computePolicyTypeVmHostAffinity),
					resource.TestCheckResourceAttr(testAccCheckVSphereComputePolicyResourceName, "policy_type", computePolicyTypeVmHostAffinity),
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
