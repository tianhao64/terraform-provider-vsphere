package vsphere

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/datastore"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"golang.org/x/net/context"
)

func resourceVSphereFile() *schema.Resource {
	return &schema.Resource{
		Create:        resourceVSphereFileCreate,
		Read:          resourceVSphereFileRead,
		Update:        resourceVSphereFileUpdate,
		Delete:        resourceVSphereFileDelete,
		CustomizeDiff: resourceVSphereFileCustomizeDiff,
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
	}
}

func resourceVSphereFileCreate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning create", d.Get("destination_file").(string))
	client := meta.(*VSphereClient).vimClient
	sourceDS, destDS, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	destFile := d.Get("destination_file").(string)
	sourceFile := d.Get("source_file").(string)
	if sourceDS != nil && d.Get("source_file").(string) != "" {
		err = fileCopy(sourceDS, sourceFile, destDS, destFile, client)
		if err != nil {
			return err
		}
	} else {
		log.Printf("[DEBUG] %s: Uploading file", d.Get("destination_file").(string))
		log.Printf("[DEBUG] %s: Uploading file", destDS)
		url := destDS.NewURL(destFile)
		log.Printf("[DEBUG] %s: Uploading file", url)
		err = client.Client.UploadFile(context.TODO(), sourceFile, url, nil)
		if err != nil {
			return err
		}
	}
	d.SetId(destFile)
	log.Printf("[DEBUG] %s: Creation completed", d.Id())
	return nil
}

func resourceVSphereFileRead(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning read", d.Id())
	client := meta.(*VSphereClient).vimClient
	_, destDS, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	destFile := d.Get("destination_file").(string)
	_, err = destDS.Stat(context.TODO(), destFile)
	if err != nil {
		if _, ok := err.(object.DatastoreNoSuchFileError); ok {
			log.Printf("[DEBUG] %s: File not found. Removing.", d.Id())
			d.SetId("")
		} else {
			return err
		}
	}
	// Since the Id is based on the destination file name, it needs to be updated if the file moves.
	if destFile != d.Id() {
		log.Printf("[DEBUG] %s: New destination file name. Updating ID to: %s", d.Id(), destFile)
		d.SetId(destFile)
	}
	log.Printf("[DEBUG] %s: Read complete", d.Id())
	return nil
}

func resourceVSphereFileUpdate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] %s: Beginning update", d.Id())
	client := meta.(*VSphereClient).vimClient
	// Since source* elements are all ForceNew, we don't need to worry about them in an update.
	_, oldDestDS, err := fileOldDatastores(d, client)
	if err != nil {
		return err
	}
	_, destDS, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	if oldDestDS == nil {
		oldDestDS = destDS
	}
	destFile := d.Get("destination_file").(string)
	oldDestFile, _ := d.GetChange("destination_file")
	if oldDestFile == nil {
		oldDestFile = destFile
	}
	oldDC, err := getDatacenter(client, oldDestDS.DatacenterPath)
	if err != nil {
		return err
	}
	destDC, err := getDatacenter(client, destDS.DatacenterPath)
	if err != nil {
		return err
	}

	fm := object.NewFileManager(client.Client)
	log.Printf("[DEBUG] %s: Moving file to: [ %s ]%s", d.Id(), destDS.Name(), destDS.Path(destFile))
	task, err := fm.MoveDatastoreFile(context.TODO(), oldDestDS.Path(oldDestFile.(string)), oldDC, destDS.Path(destFile), destDC, true)
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
	_, destDS, err := fileDatastores(d, client)
	if err != nil {
		return err
	}
	destFile := d.Get("destination_file").(string)
	fm := object.NewFileManager(client.Client)
	destDC, _ := getDatacenter(client, destDS.DatacenterPath)
	task, err := fm.DeleteDatastoreFile(context.TODO(), destDS.Path(destFile), destDC)
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

func resourceVSphereFileCustomizeDiff(d *schema.ResourceDiff, _ interface{}) error {
	// Check that enough info is provided for the destination file. The minimum requirement is either a datacenter or
	// datastore id. If just a datacenter is provided, then the default datastore will be used for that datacenter. When
	// a datastore_id is provided, no datacenter is required since it is a unique identifier.
	_, dcOk := d.GetOk("datacenter")
	_, dsIdOk := d.GetOk("datastore_id")
	if !dcOk && !dsIdOk {
		return fmt.Errorf("datacenter or datastore_id required for vsphere_file resources")
	}
	return nil
}

func fileOldDatastores(d *schema.ResourceData, c *govmomi.Client) (*object.Datastore, *object.Datastore, error) {
	log.Printf("[DEBUG] %s: Looking up old source and destination datastores", d.Id())
	// Get the old destination datastore and datacenter info.
	oddsn, _ := d.GetChange("datastore")
	oddcn, _ := d.GetChange("datacenter")
	oddsi, _ := d.GetChange("datastore_id")
	dds, err := fileDatastore(oddsn.(string), oddcn.(string), oddsi.(string), c)
	log.Printf("[DEBUG] %s: Old destination datastore found: %s", d.Id(), dds.Name())
	if err != nil {
		return nil, nil, err
	}
	// Get the old source datastore and datacenter info.
	var sds *object.Datastore
	osdsn, _ := d.GetChange("source_datastore")
	osdcn, _ := d.GetChange("source_datacenter")
	osdsi, _ := d.GetChange("source_datastore_id")
	if osdsi.(string) != "" || osdcn.(string) != "" {
		sds, err = fileDatastore(osdsn.(string), osdcn.(string), osdsi.(string), c)
		if err != nil {
			return nil, nil, err
		}
		log.Printf("[DEBUG] %s: Old source datastore found: %s", d.Id(), sds.Name())
	}
	log.Printf("[DEBUG] %s: Old source and destination datastore lookup complete", d.Id())
	return dds, sds, nil
}

func fileDatastores(d *schema.ResourceData, c *govmomi.Client) (*object.Datastore, *object.Datastore, error) {
	log.Printf("[DEBUG] %s: Looking up source and destination datastores", d.Id())
	// Get the destination datastore and datacenter info.
	ddsi := d.Get("datastore_id").(string)
	ddsn := d.Get("datastore").(string)
	ddcn := d.Get("datacenter").(string)
	dds, err := fileDatastore(ddsn, ddcn, ddsi, c)
	log.Printf("[DEBUG] %s: Destination datastore found: %s", d.Id(), dds.Name())
	if err != nil {
		return nil, nil, err
	}
	log.Printf("[DEBUG] fileDatastores: Found destination datastore %s", dds.Name())
	// Get the source datastore and datacenter info.
	var sds *object.Datastore
	sdsi := d.Get("source_datastore_id").(string)
	sdsn := d.Get("source_datastore").(string)
	sdcn := d.Get("source_datacenter").(string)
	if sdsi != "" || sdcn != "" {
		sds, err = fileDatastore(sdsn, sdcn, sdsi, c)
		if err != nil {
			return nil, nil, err
		}
		log.Printf("[DEBUG] %s: Source datastore found: %s", d.Id(), sds.Name())
	}
	log.Printf("[DEBUG] %s: Source and destination datastore lookup complete", d.Id())
	return sds, dds, nil
}

// fileDatastore will no longer be needed after datacenter and datacenter names are removed in favor of datastore_id.
func fileDatastore(datastoreName string, datacenterName string, datastoreId string, c *govmomi.Client) (*object.Datastore, error) {
	dc, err := getDatacenter(c, datacenterName)
	if err != nil {
		return nil, err
	}
	switch {
	case datastoreId == "" && datastoreName != "":
		return datastore.FromPath(c, datastoreName, dc)
	case datastoreId != "":
		return datastore.FromID(c, datastoreId)
	}
	return datastore.DefaultDatastore(c, dc)
}

func createDir(file string, ds *object.Datastore, c *govmomi.Client) error {
	log.Printf("[DEBUG] %s: Creating directory", file)
	fm := object.NewFileManager(c.Client)
	di := strings.LastIndex(file, "/")
	if di == -1 {
		return nil
	}
	ddc, _ := getDatacenter(c, ds.DatacenterPath)
	path := file[0:di]
	err := fm.MakeDirectory(context.TODO(), ds.Path(path), ddc, true)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] %s: Directory created", file)
	return nil
}

func fileCopy(sds *object.Datastore, sf string, dds *object.Datastore, df string, c *govmomi.Client) error {
	log.Printf("[DEBUG] fileCopy: Copying file: [%s] %s to: [%s] %s", sds.Name(), sf, dds.Name(), df)
	err := createDir(df, dds, c)
	if err != nil {
		return err
	}
	sdc, _ := getDatacenter(c, sds.DatacenterPath)
	ddc, _ := getDatacenter(c, dds.DatacenterPath)
	log.Printf("[DEBUG] fileCopy: Source path: %s, Destination path: %s", sds.Path(sf), dds.Path(df))
	re := regexp.MustCompile(".*\\.vmdk$")
	var task *object.Task
	if re.Match([]byte(df)) {
		log.Printf("[DEBUG] fileCopy: File appears to be a VMDK. Using VirtualDiskManager")
		vdm := object.NewVirtualDiskManager(c.Client)
		task, err = vdm.CopyVirtualDisk(context.TODO(), sds.Path(sf), sdc, dds.Path(df), ddc, nil, true)
	} else {
		log.Printf("[DEBUG] fileCopy: File is not a VMDK. Using FileManager")
		fm := object.NewFileManager(c.Client)
		task, err = fm.CopyDatastoreFile(context.TODO(), sds.Path(sf), sdc, dds.Path(df), ddc, true)
	}
	if err != nil {
		return err
	}
	_, err = task.WaitForResult(context.TODO(), nil)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] fileCopy: File copy complete")
	return nil
}

func fileDeprecationNotice(old string, current string) string {
	return fmt.Sprintf(`
The %q attribute for files will be removed in favor of %q in
future releases. To transition existing files, rename the deprecated attribute to
the new. When doing so, ensure the value of the attribute stays the same.
`, old, current)
}
