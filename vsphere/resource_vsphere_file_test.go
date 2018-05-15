package vsphere

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/datastore"
	"github.com/vmware/govmomi/object"
	"golang.org/x/net/context"
)

func TestAccResourceVSphereFile_basic(t *testing.T) {
	fileName := "/tmp/tf_file"
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccResourceVSphereFilePreCheck(t)
			testAccResourceVSphereFileCreateFile(fileName)
		},
		Providers:    testAccProviders,
		CheckDestroy: testAccResourceVSphereFileCheckExists(false),
		Steps: []resource.TestStep{
			{
				Config: testAccResourceVSphereFileConfigBasic(fileName),
				Check: resource.ComposeTestCheckFunc(
					testAccResourceVSphereFileCheckExists(true),
				),
			},
		},
	})
}

func TestAccResourceVSphereFile_namesNotIDs(t *testing.T) {
	fileName := "/tmp/tf_file"
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccResourceVSphereFilePreCheck(t)
			testAccResourceVSphereFileCreateFile(fileName)
		},
		Providers:    testAccProviders,
		CheckDestroy: testAccResourceVSphereFileCheckExists(false),
		Steps: []resource.TestStep{
			{
				Config: testAccResourceVSphereFileConfigBasicNames(fileName),
				Check: resource.ComposeTestCheckFunc(
					testAccResourceVSphereFileCheckExists(true),
				),
			},
		},
	})
}

func testAccResourceVSphereFileCheckExists(expected bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["vsphere_file.file"]
		if !ok {
			if expected {
				return fmt.Errorf("Resource not found: file")
			} else {
				return nil
			}
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		client := testAccProvider.Meta().(*VSphereClient).vimClient
		dc, _ := getDatacenter(client, rs.Primary.Attributes["datacenter"])
		dsID := rs.Primary.Attributes["datastore_id"]
		dsName := rs.Primary.Attributes["datastore"]
		var ds *object.Datastore
		var err error
		switch {
		case dsID != "":
			ds, err = datastore.FromID(client, dsID)
			if err != nil {
				return err
			}
		case dsName == "":
			ds, err = datastore.DefaultDatastore(client, dc)
			if err != nil {
				return err
			}
		case dsName != "" && dc != nil:
			ds, err = datastore.FromPath(client, dsName, dc)
			if err != nil {
				return err
			}
		}
		_, err = ds.Stat(context.TODO(), rs.Primary.Attributes["destination_file"])
		if err != nil {
			switch e := err.(type) {
			case object.DatastoreNoSuchFileError:
				if expected {
					return fmt.Errorf("File does not exist: %s", e.Error())
				}
				return nil
			default:
				return err
			}
		}
		return nil
	}
}

func testAccResourceVSphereFileCreateFile(name string) error {
	err := ioutil.WriteFile(name, []byte("emptyData"), 0644)
	if err != nil {
		return err
	}
	return nil
}

func testAccResourceVSphereFilePreCheck(t *testing.T) {
	if os.Getenv("VSPHERE_DATACENTER") == "" {
		t.Skip("set VSPHERE_DATACENTER to run vsphere_file acceptance tests")
	}
	if os.Getenv("VSPHERE_RESOURCE_POOL") == "" {
		t.Skip("set VSPHERE_RESOURCE_POOL to run vsphere_file acceptance tests")
	}
	if os.Getenv("VSPHERE_DATASTORE") == "" {
		t.Skip("set VSPHERE_DATASTORE to run vsphere_file acceptance tests")
	}
	if os.Getenv("VSPHERE_ESXI_HOST") == "" {
		t.Skip("set VSPHERE_ESXI_HOST to run vsphere_file acceptance tests")
	}
	if os.Getenv("VSPHERE_DS_VMFS_DISK0") == "" {
		t.Skip("set VSPHERE_DS_VMFS_DISK0 to run vsphere_file acceptance tests")
	}
}

func testAccResourceVSphereFileConfigBasic(sourceFile string) string {
	return fmt.Sprintf(`
variable "datacenter" {
	default = "%s"
}

variable "datastore" {
	default = "%s"
}

variable "destination_file" {
	default = "/terraform_test_file_basic"
}

variable "source_file" {
	default = "%s"
}

data "vsphere_datacenter" "datacenter" {
	name = "${var.datacenter}"
}

data "vsphere_datastore" "datastore" {
	name = "${var.datastore}"
	datacenter_id = "${data.vsphere_datacenter.datacenter.id}"
}

resource "vsphere_file" "file" {
	destination_file = "${var.destination_file}"
	datastore_id = "${data.vsphere_datastore.datastore.id}"
	source_file = "${var.source_file}"
}
`,
		os.Getenv("VSPHERE_DATACENTER"),
		os.Getenv("VSPHERE_DATASTORE"),
		sourceFile,
	)
}

func testAccResourceVSphereFileConfigBasicNames(sourceFile string) string {
	return fmt.Sprintf(`
variable "datacenter" {
	default = "%s"
}

variable "datastore" {
	default = "%s"
}

variable "destination_file" {
	default = "/terraform_test_file_basic"
}

variable "source_file" {
	default = "%s"
}

data "vsphere_datacenter" "datacenter" {
	name = "${var.datacenter}"
}

data "vsphere_datastore" "datastore" {
	name = "${var.datastore}"
	datacenter_id = "${data.vsphere_datacenter.datacenter.id}"
}

resource "vsphere_file" "file" {
	destination_file = "${var.destination_file}"
	datastore = "${var.datastore}"
	datacenter = "${var.datacenter}"
	source_file = "${var.source_file}"
}
`,
		os.Getenv("VSPHERE_DATACENTER"),
		os.Getenv("VSPHERE_DATASTORE"),
		sourceFile,
	)
}
