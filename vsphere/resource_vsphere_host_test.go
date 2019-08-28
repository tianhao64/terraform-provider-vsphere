package vsphere

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware/govmomi"

	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/hostsystem"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccResourceVSphereHost_basic(t *testing.T) {

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			// Test if host specific env vars are set
			// testAccVSpherePreLicenseBasicCheck(t)
		},
		Providers: testAccProviders,
		// CheckDestroy: testAccVSphereHostDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVSphereHostConfig(),
				Check: resource.ComposeTestCheckFunc(
					testAccVSphereHostExists("vsphere_host.h1"),
				),
			},
		},
	})

}

func testAccVSphereHostConfig() string {
	return fmt.Sprintf(`
	data "vsphere_datacenter" "dc" {
	  name = "%s"
	}
		
	data "vsphere_compute_cluster" "c1" {
	  name = "%s"
	  datacenter_id = data.vsphere_datacenter.dc.id
	}
		
	resource "vsphere_host" "h1" {
	  # Useful only for connection
	  hostname = "%s"
	  username = "%s"
	  password = "%s"
	  thumbprint = "%s"
	
	  # Makes sense to update
	  license = "%s"
	  cluster = data.vsphere_compute_cluster.c1.id
	}	  
	`, os.Getenv("VSPHERE_DATACENTER"),
		os.Getenv("VSPHERE_CLUSTER"),
		os.Getenv("ESX_HOSTNAME"),
		os.Getenv("ESX_USERNAME"),
		os.Getenv("ESX_PASSWORD"),
		os.Getenv("ESX_THUMBPRINT"),
		os.Getenv("VSPHERE_LICENSE"))
}

func hostExists(client *govmomi.Client, hostID string) (bool, error) {
	hs, err := hostsystem.FromID(client, hostID)
	if err != nil {
		if soap.IsSoapFault(err) {
			sf := soap.ToSoapFault(err)
			_, ok := sf.Detail.Fault.(types.ManagedObjectNotFound)
			if !ok {
				return false, err
			}
			return false, nil
		}
		return false, err
	}

	if strings.Split(hs.String(), ":")[1] != hostID {
		return false, nil
	}
	return true, nil
}

func testAccVSphereHostExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("%s key not found on the server", name)
		}
		hostID := rs.Primary.ID
		client := testAccProvider.Meta().(*VSphereClient).vimClient
		res, err := hostExists(client, hostID)
		if err != nil {
			return err
		}

		if !res {
			return fmt.Errorf("Host with ID %s not found", hostID)
		}

		return nil
	}
}

func testAccVSphereHostDestroy(s *terraform.State) error {
	message := ""
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vsphere_host" {
			continue
		}
		hostID := rs.Primary.ID
		client := testAccProvider.Meta().(*VSphereClient).vimClient
		res, err := hostExists(client, hostID)
		if err != nil {
			return err
		}

		if !res {
			message += fmt.Sprintf("Host with ID %s not found", hostID)
		}
	}
	if message != "" {
		return errors.New(message)
	}
	return nil
}
