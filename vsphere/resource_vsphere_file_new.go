package vsphere

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/datastore"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/soap"
	"golang.org/x/net/context"
)

func resourceVSphereFile() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"datacenter": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"datacenter_id"},
				Description:   "Name of the datacenter in which the destination datastore resides.",
				Deprecated:    fileDeprecationNotice("datacenter", "datastore_id"),
			},
			"source_datacenter": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"source_datacenter_id"},
				Description:   "Name of the datacenter in which the source datastore resides.",
				Deprecated:    fileDeprecationNotice("source_datacenter", "source_datastore_id"),
			},
			"datastore": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"datastore_id"},
				Description:   "The name of the destination file's datastore.",
				Deprecated:    fileDeprecationNotice("datastore", "datastore_id"),
			},
			"source_datastore": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"source_datastore_id"},
				Description:   "The name of the source file's datastore.",
				Deprecated:    fileDeprecationNotice("source_datastore", "source_datastore_id"),
			},
			"datastore_id": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"datastore", "datacenter"},
				Description:   "The ID of the destination file's datastore.",
			},
			"source_datastore_id": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"source_datastore", "source_datacenter"},
				Description:   "The ID of the source file's datastore.",
			},
			"source_file": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The path and name of the source file.",
			},
			"destination_file": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The path and name of the destination file.",
			},
			"create_directories": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Indicates of non-existing directories should be created for destination file.",
			},
		},
		Create: resourceVSphereFileCreate,
		Read:   resourceVSphereFileRead,
		Update: resourceVSphereFileUpdate,
		Delete: resourceVSphereFileDelete,
	}
}

func resourceVSphereFileCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	sds, dds, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	if(sds != nil && d.Get("source_filename").(string) != "" {
	}
}

func fileDatastores(d *schema.ResourceData, c *govmomi.Client) (*object.Datastore, *object.Datastore, error) {
	var sds *object.Datastore
	var dds *object.Datastore
	var err error
	// Get the destination datastore
	if dsID, ok := d.GetOk("datastore_id"); ok {
		dds, err = datastore.FromID(c, dsID.(string))
	} else {
		dsName := d.Get("datastore").(string)
		dcName := d.Get("datacenter").(string)
		dds, err = fileDatastore(dsName, dcName, c)
	}
	if err != nil {
		return nil, nil, err
	}
	// Get the source datastore
	_, iok := d.GetOk("source_datastore_id")
	_, nok := d.GetOk("source_datastore")
	switch {
	case iok:
		sds, err = datastore.FromID(c, d.Get("datastore_id").(string))
	case nok:
		dsName := d.Get("source_datastore").(string)
		dcName := d.Get("source_datacenter").(string)
		sds, err = fileDatastore(dsName, dcName, c)
	}
	if err != nil {
		return nil, nil, err
	}
	return sds, dds, nil
}

func copyFile(sds *object.Datastore, sf string, dds *object.Datastore, df string) {
	fm := object.NewFileManager(client.Client)
	if f.createDirectories {
		directoryPathIndex := strings.LastIndex(f.destinationFile, "/")
		path := f.destinationFile[0:directoryPathIndex]
		err = fm.MakeDirectory(context.TODO(), ds.Path(path), dc, true)
		if err != nil {
			return fmt.Errorf("error %s", err)
		}
	}
	task, err := fm.CopyDatastoreFile(context.TODO(), sds.Path(f.sdf), nil, dds.Path(f.df), nil, true)
}

func fileDatastore(dsName string, dcName string, client *govmomi.Client) (*object.Datastore, error) {
	dc, err := getDatacenter(client, dcName)
	if err != nil {
		return err
	}
	return datastore.FromPath(dsName, dc, client)
}

func fileDeprecationNotice(old string, current string) string {
	return fmt.Sprintf(`
The %q attribute for files will be removed in favor of %q in
future releases. To transition existing files, rename the deprecated attribute to
the new. When doing so, ensure the value of the attribute stays the same.
`, old, current)
}
