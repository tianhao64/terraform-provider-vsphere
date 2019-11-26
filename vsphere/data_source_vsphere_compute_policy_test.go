package vsphere

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
)

var testAccDataSourceVSphereComputePolicyExpectedRegexp = regexp.MustCompile("^policy")

func TestAccDataSourceVSphereComputePolicy(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccDataSourceVSphereComputePolicyPreCheck(t)
		},
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceVSphereComputePolicyConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"data.vsphere_compuate_policy.policy",
						"id",
						testAccDataSourceVSphereComputePolicyExpectedRegexp,
					),
				),
			},
		},
	})
}

func testAccDataSourceVSphereComputePolicyPreCheck(t *testing.T) {
	if os.Getenv("VSPHERE_COMPUTE_POLICY") == "" {
		t.Skip("set VSPHERE_COMPUTE_POLICY to run vsphere_compuate_policy acceptance tests")
	}
}

func testAccDataSourceVSphereComputePolicyConfig() string {
	return fmt.Sprintf(`
data "vsphere_compuate_policy" "policy" {
  name = "%s"
}
`, os.Getenv("VSPHERE_COMPUTE_POLICY"))
}
