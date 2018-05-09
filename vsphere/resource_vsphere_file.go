package vsphere

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/datastore"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"golang.org/x/net/context"
)

func resourceVSphereFile() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"datacenter": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"datastore_id"},
				Description:   "Name of the datacenter in which the destination datastore resides.",
				Deprecated:    fileDeprecationNotice("datacenter", "datastore_id"),
			},
			"source_datacenter": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"source_datastore_id"},
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
				Deprecated:  "create_directories is deprecated. Missing parent directories will automatically be created.",
			},
		},
		Create: resourceVSphereFileCreate,
		Read:   resourceVSphereFileRead,
		Update: resourceVSphereFileUpdate,
		Delete: resourceVSphereFileDelete,
	}
}

func resourceVSphereFileRead(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning read", d.Id())
	client := meta.(*VSphereClient).vimClient
	_, dds, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	df := d.Get("destination_file").(string)
	_, err = dds.Stat(context.TODO(), df)
	if err != nil {
		if _, ok := err.(object.DatastoreNoSuchFileError); ok {
			log.Printf("[DEBUG] %s: File not found. Removing.", d.Id())
			d.SetId("")
		} else {
			return err
		}
	}
	log.Printf("[DEBUG] %s: Read complete", d.Id())
	return nil
}

func resourceVSphereFileUpdate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning update", d.Id())
	client := meta.(*VSphereClient).vimClient
	_, odds, err := fileOldDatastores(d, client)
	if err != nil {
		return err
	}
	_, dds, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	if odds == nil {
		odds = dds
	}
	df := d.Get("destination_file").(string)
	odf, _ := d.GetChange("destination_file")
	if odf == nil {
		odf = df
	}
	odc, err := getDatacenter(client, odds.DatacenterPath)
	if err != nil {
		return err
	}
	dc, err := getDatacenter(client, dds.DatacenterPath)
	if err != nil {
		return err
	}

	fm := object.NewFileManager(client.Client)
	log.Printf("[DEBUG] %s: Moving file from: %s, to: %s", d.Id(), odds.Path(odf.(string)), dds.Path(df))
	task, err := fm.MoveDatastoreFile(context.TODO(), odds.Path(odf.(string)), odc, dds.Path(df), dc, true)
	if err != nil {
		return err
	}
	_, err = task.WaitForResult(context.TODO(), nil)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] %s: Update complete", d.Id())
	return nil
}

func resourceVSphereFileDelete(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning delete", d.Id())
	client := meta.(*VSphereClient).vimClient
	_, dds, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	df := d.Get("destination_file").(string)
	fm := object.NewFileManager(client.Client)
	dc, _ := getDatacenter(client, dds.DatacenterPath)
	task, err := fm.DeleteDatastoreFile(context.TODO(), dds.Path(df), dc)
	if err != nil {
		return err
	}
	_, err = task.WaitForResult(context.TODO(), nil)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] %s: Delete complete", d.Id())
	return nil
}

func resourceVSphereFileCreate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning create", d.Get("destination_file").(string))
	client := meta.(*VSphereClient).vimClient
	sds, dds, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	df := d.Get("destination_file").(string)
	sf := d.Get("source_file").(string)
	if sds != nil && d.Get("source_file").(string) != "" {
		err = fileCopy(sds, sf, dds, df, client)
		if err != nil {
			return err
		}
	} else {
		url := dds.NewURL(df)
		err = client.Client.UploadFile(context.TODO(), sf, url, nil)
		if err != nil {
			return err
		}
	}
	d.SetId(fmt.Sprintf("[%s] %s", dds.Reference().Value, df))
	log.Printf("[DEBUG] %s: Creation completed", d.Id())
	return nil
}

func fileOldDatastores(d *schema.ResourceData, c *govmomi.Client) (*object.Datastore, *object.Datastore, error) {
	var sds *object.Datastore
	var dds *object.Datastore
	oddsn, _ := d.GetChange("datastore")
	oddcn, _ := d.GetChange("datacenter")
	oddsi, _ := d.GetChange("datastore_id")
	dds, err := fileDatastore(oddsn.(string), oddcn.(string), oddsi.(string), c)
	if err != nil {
		return nil, nil, err
	}

	osdsn, _ := d.GetChange("source_datastore")
	osdcn, _ := d.GetChange("source_datacenter")
	osdsi, _ := d.GetChange("source_datastore_id")
	if osdsi.(string) != "" || osdsn.(string) != "" {
		sds, err = fileDatastore(osdsn.(string), osdcn.(string), osdsi.(string), c)
		if err != nil {
			return nil, nil, err
		}
	}
	return dds, sds, nil
}

func fileDatastores(d *schema.ResourceData, c *govmomi.Client) (*object.Datastore, *object.Datastore, error) {
	var sds *object.Datastore
	var dds *object.Datastore
	// Get the destination datastore
	ddsi := d.Get("datastore_id").(string)
	ddsn := d.Get("datastore").(string)
	ddcn := d.Get("datacenter").(string)
	dds, err := fileDatastore(ddsn, ddcn, ddsi, c)
	if err != nil {
		return nil, nil, err
	}
	// Get the source datastore
	sdsi := d.Get("source_datastore_id").(string)
	sdsn := d.Get("source_datastore").(string)
	sdcn := d.Get("source_datacenter").(string)
	if sdsi != "" || sdsn != "" {
		sds, err = fileDatastore(sdsn, sdcn, sdsi, c)
		if err != nil {
			return nil, nil, err
		}
	}
	return sds, dds, nil
}

func fileDatastore(dsn string, dcn string, dsi string, c *govmomi.Client) (*object.Datastore, error) {
	if dsi == "" {
		dc, err := getDatacenter(c, dcn)
		if err != nil {
			return nil, err
		}
		return datastore.FromPath(c, dsn, dc)
	}
	return datastore.FromID(c, dsi)
}

func fileCreateDir(df string, dds *object.Datastore, fm *object.FileManager) error {
	di := strings.LastIndex(df, "/")
	if di == -1 {
		return nil
	}
	path := df[0:di]
	err := fm.MakeDirectory(context.TODO(), dds.Path(path), nil, true)
	if err != nil {
		return err
	}
	return nil
}

func fileCopy(sds *object.Datastore, sf string, dds *object.Datastore, df string, c *govmomi.Client) error {
	fm := object.NewFileManager(c.Client)
	err := fileCreateDir(df, dds, fm)
	if err != nil {
		return err
	}
	sdc, _ := getDatacenter(c, sds.DatacenterPath)
	ddc, _ := getDatacenter(c, sds.DatacenterPath)
	task, err := fm.CopyDatastoreFile(context.TODO(), sds.Path(sf), sdc, dds.Path(df), ddc, true)
	if err != nil {
		return err
	}
	_, err = task.WaitForResult(context.TODO(), nil)
	if err != nil {
		return err
	}
	return nil
}

func fileDeprecationNotice(old string, current string) string {
	return fmt.Sprintf(`
The %q attribute for files will be removed in favor of %q in
future releases. To transition existing files, rename the deprecated attribute to
the new. When doing so, ensure the value of the attribute stays the same.
`, old, current)
}
