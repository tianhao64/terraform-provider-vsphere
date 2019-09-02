package vsphere

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/hostsystem"
	"github.com/vmware/govmomi/license"

	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/clustercomputeresource"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceVsphereHost() *schema.Resource {
	return &schema.Resource{
		Create: resourceVsphereHostCreate,
		Read:   resourceVsphereHostRead,
		Update: resourceVsphereHostUpdate,
		Delete: resourceVsphereHostDelete,
		// Importer: ,
		Schema: map[string]*schema.Schema{
			"cluster": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "ID of the vSphere cluster the host will belong to.",
			},
			"hostname": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "FQDN or IP address of the host.",
			},
			"username": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Username of the administration account of the host.",
			},
			"password": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Password of the administration account of the host.",
				Sensitive:   true,
			},
			"thumbprint": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Host's certificate SHA-1 thumbprint. If not set then the CA that signed the host's certificate must be trusted.",
			},
			"license": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "License key that will be applied to this host.",
			},
			"force": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Force add the host to vsphere, even if it's already managed by a different vSphere instance.",
				Default:     false,
			},
			"connected": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Set the state of the host. If set to false then the host will be asked to disconnect.",
				Default:     true,
			},
		},
	}
}

func resourceVsphereHostRead(d *schema.ResourceData, meta interface{}) error {

	// NOTE: Destroying the host without telling vsphere about it will result in us not
	// knowing that the host does not exist any more.

	// Look for host
	client := meta.(*VSphereClient).vimClient
	hostID := d.Id()
	if hostID == "" {
		return nil
	}

	// Find host and get reference to it.
	hs, err := hostsystem.FromID(client, hostID)
	if err != nil {
		if soap.IsSoapFault(err) {
			sf := soap.ToSoapFault(err)
			_, ok := sf.Detail.Fault.(types.ManagedObjectNotFound)
			if !ok {
				log.Printf("[DEBUG] Error while searching host %s. Error: %s", hostID, err)
				return err
			}
			d.SetId("")
			return nil
		}
		log.Printf("[DEBUG] non SOAP error while searching host %s. Error %#v", hostID, err)
		return err
	}

	// Retrieve host's properties.
	log.Printf("[DEBUG] Got host %#v", hs)
	host, err := hostsystem.Properties(hs)
	if err != nil {
		log.Printf("[DEBUG] Error while retrieving properties for host %s. Error: %s", hostID, err)
		return err
	}

	if host.Parent != nil {
		d.Set("cluster", host.Parent.Value)
	} else {
		d.Set("cluster", "")
	}

	connectionState, err := hostsystem.GetConnectionState(hs)
	if err != nil {
		return err
	}

	if connectionState != types.HostSystemConnectionStateDisconnected {
		d.Set("connected", true)
	} else {
		d.Set("conencted", false)
	}

	lm := license.NewManager(client.Client)
	am, err := lm.AssignmentManager(context.TODO())
	if err != nil {
		return err
	}
	licenses, err := am.QueryAssigned(context.TODO(), hostID)
	if err != nil {
		return err
	}

	licenseKey := d.Get("license").(string)
	licFound := false
	for _, lic := range licenses {
		if licenseKey == lic.AssignedLicense.LicenseKey {
			licFound = true
			break
		}
	}

	if !licFound {
		d.Set("license", "")
	}

	return nil
}

func resourceVsphereHostCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient

	clusterID := d.Get("cluster").(string)
	ccr, err := clustercomputeresource.FromID(client, clusterID)
	if err != nil {
		log.Printf("[DEBUG] Error while searching cluster %s. Error: %s", clusterID, err)
		return err
	}

	hcs := types.HostConnectSpec{
		HostName:      d.Get("hostname").(string),
		UserName:      d.Get("username").(string),
		Password:      d.Get("password").(string),
		SslThumbprint: d.Get("thumbprint").(string),
		Force:         d.Get("force").(bool),
	}

	licenseKey := d.Get("license").(string)

	lm := license.NewManager(client.Client)
	ll, err := lm.List(context.TODO())
	if err != nil {
		return err
	}

	licFound := false
	for _, l := range ll {
		if l.LicenseKey == licenseKey {
			licFound = true
			break
		}
	}
	if !licFound {
		return fmt.Errorf("license key supplied (%s) did not match against known license keys", licenseKey)
	}

	forcedState := d.Get("forced").(bool)
	task, err := ccr.AddHost(context.TODO(), hcs, forcedState, &licenseKey, nil)
	if err != nil {
		log.Printf("[DEBUG] Error while adding host with hostname %s to cluster %s.  Error: %s", d.Get("hostname").(string), clusterID, err)
	}
	task.Wait(context.TODO())

	var to mo.Task
	err = task.Properties(context.TODO(), task.Reference(), nil, &to)
	if err != nil {
		log.Printf("[DEBUG] Failed while getting task results: %s", err)
		return err
	}

	if to.Info.State != "success" {
		return fmt.Errorf("Host addition failed. %s", to.Info.Error)
	}
	hostID := strings.Split(to.Info.Result.(types.ManagedObjectReference).String(), ":")[1]
	d.SetId(hostID)
	log.Printf("[DEBUG] set host ID to %s", hostID)

	return resourceVsphereHostRead(d, meta)
}

func resourceVsphereHostUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient

	// First let's establish where we are and where we want to go
	var desiredConnectionState bool
	if d.HasChange("connected") {
		_, new := d.GetChange("connected")
		desiredConnectionState = new.(bool)
	} else {
		desiredConnectionState = d.Get("connected").(bool)
	}

	hostID := d.Id()
	hostObject, err := hostsystem.FromID(client, hostID)
	if err != nil {
		return err
	}

	actualConnectionState, err := hostsystem.GetConnectionState(hostObject)
	if err != nil {
		return err
	}

	// Have there been any changes that warrant a reconnect?
	reconnect := false
	connectionKeys := []string{"hostname", "username", "password", "thumbprint"}
	for _, k := range connectionKeys {
		if d.HasChange(k) {
			reconnect = true
			break
		}
	}

	// Decide if we're going to reconnect or not
	reconnectNeeded, err := shouldReconnect(d, meta, actualConnectionState, desiredConnectionState, reconnect)
	if err != nil {
		return err
	}

	switch reconnectNeeded {
	case 1:
		err := handleReconnect(d, meta)
		if err != nil {
			return err
		}
	case -1:
		err := handleDisconnect(d, meta)
		if err != nil {
			return err
		}
	case 0:
		break
	}

	mutableKeys := map[string]func(*schema.ResourceData, interface{}, interface{}, interface{}) error{
		"license": modifyLicense,
		"cluster": modifyCluster,
	}
	for k, v := range mutableKeys {
		if !d.HasChange(k) {
			continue
		}
		old, new := d.GetChange(k)
		err := v(d, meta, old, new)
		if err != nil {
			return fmt.Errorf("error while updating %s: %s", k, err)
		}
	}
	return resourceVsphereHostRead(d, meta)
}

func resourceVsphereHostDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	hostID := d.Id()

	hs, err := hostsystem.FromID(client, hostID)
	if err != nil {
		return err
	}

	err = hostsystem.EnterMaintenanceMode(hs, int(defaultAPITimeout), true)
	if err != nil {
		return fmt.Errorf("error while putting host to maintenance mode: %s", err.Error())
	}

	task, err := hs.Destroy(context.TODO())
	if err != nil {
		return err
	}
	err = task.Wait(context.TODO())
	if err != nil {
		return fmt.Errorf("Error while waiting for host (%s) to be removed: %s", hostID, err)
	}

	var to mo.Task
	err = task.Properties(context.TODO(), task.Reference(), nil, &to)
	if err != nil {
		log.Printf("[DEBUG] Failed while getting task results: %s", err)
		return err
	}

	if to.Info.State != "success" {
		return fmt.Errorf("Error while removing host(%s): %s", hostID, to.Info.Error)
	}
	return nil
}

func modifyLicense(d *schema.ResourceData, meta, old, new interface{}) error {
	client := meta.(*VSphereClient).vimClient
	lm := license.NewManager(client.Client)
	lam, err := lm.AssignmentManager(context.TODO())
	if err != nil {
		return err
	}
	lam.Update(context.TODO(), d.Id(), new.(string), "")
	return nil
}

func modifyCluster(d *schema.ResourceData, meta, old, new interface{}) error {
	client := meta.(*VSphereClient).vimClient
	hostID := d.Id()
	newClusterID := new.(string)

	newCluster, err := clustercomputeresource.FromID(client, newClusterID)
	if err != nil {
		log.Printf("[DEBUG] Error while searching new cluster %s. Error: %s.", newClusterID, err)
		return err
	}

	hs, err := hostsystem.FromID(client, hostID)
	if err != nil {
		return err
	}

	err = hostsystem.EnterMaintenanceMode(hs, int(defaultAPITimeout), false)
	if err != nil {
		return fmt.Errorf("error while putting host to maintenance mode: %s", err.Error())
	}

	task, err := newCluster.MoveInto(context.TODO(), hs)
	if err != nil {
		return err
	}
	task.Wait(context.TODO())

	err = hostsystem.ExitMaintenanceMode(hs, int(defaultAPITimeout))
	if err != nil {
		return fmt.Errorf("error while taking host out of maintenance mode: %s", err.Error())
	}

	var to mo.Task
	err = task.Properties(context.TODO(), task.Reference(), nil, &to)
	if err != nil {
		log.Printf("[DEBUG] Failed while getting task results: %s", err)
		return err
	}

	if to.Info.State != "success" {
		return fmt.Errorf("Error while moving host to new cluster (%s): %s", newClusterID, to.Info.Error)
	}

	return nil
}

func handleReconnect(d *schema.ResourceData, meta interface{}) error {
	hostID := d.Id()
	client := meta.(*VSphereClient).vimClient
	host := object.NewHostSystem(client.Client, types.ManagedObjectReference{Type: "HostSystem", Value: d.Id()})
	hcs := types.HostConnectSpec{
		HostName:      d.Get("hostname").(string),
		UserName:      d.Get("username").(string),
		Password:      d.Get("password").(string),
		SslThumbprint: d.Get("thumbprint").(string),
		Force:         d.Get("force").(bool),
	}

	task, err := host.Reconnect(context.TODO(), &hcs, nil)
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return fmt.Errorf("Error while waiting for host (%s) to reconnect: %s", hostID, err)
	}

	var to mo.Task
	err = task.Properties(context.TODO(), task.Reference(), nil, &to)
	if err != nil {
		log.Printf("[DEBUG] Failed while getting task results: %s", err)
		return err
	}

	if to.Info.State != "success" {
		return fmt.Errorf("Error while reconnecting host(%s): %s", hostID, to.Info.Error)
	}
	return nil
}

func handleDisconnect(d *schema.ResourceData, meta interface{}) error {
	hostID := d.Id()
	client := meta.(*VSphereClient).vimClient
	host := object.NewHostSystem(client.Client, types.ManagedObjectReference{Type: "HostSystem", Value: d.Id()})
	task, err := host.Disconnect(context.TODO())
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	if err != nil {
		return fmt.Errorf("Error while waiting for host (%s) to disconnect: %s", hostID, err)
	}

	var to mo.Task
	err = task.Properties(context.TODO(), task.Reference(), nil, &to)
	if err != nil {
		log.Printf("[DEBUG] Failed while getting task results: %s", err)
		return err
	}

	if to.Info.State != "success" {
		return fmt.Errorf("Error while disconnecting host(%s): %s", hostID, to.Info.Error)
	}
	return nil
}

func shouldReconnect(d *schema.ResourceData, meta interface{}, actual types.HostSystemConnectionState, desired, shouldReconnect bool) (int, error) {
	log.Printf("[DEBUG] Figuring out if we need to do something about the host's connection")

	// desired state is connected and one of the connectionKeys has changed
	if shouldReconnect && desired {
		log.Printf("[DEBUG] Desired state is connected and one of the settings relevant to the connection changed. Reconnecting")
		return 1, nil
	}

	// desired state is connected and actual state is disconnected
	if desired && (actual != types.HostSystemConnectionStateConnected) {
		log.Printf("[DEBUG] Desired state is connected but host is not connected. Reconnecting")
		return 1, nil
	}

	// desired state is connected and actual state is connected (or host is missing heartbeats) and
	// none of the connectionKeys have changed.
	if desired && (actual != types.HostSystemConnectionStateDisconnected) && !shouldReconnect {
		log.Printf("[DEBUG] Desired state is connected and host is connected and no changes in config. Noop")
		return 0, nil
	}

	// desired state is disconnected and host is disconnected
	if !desired && (actual == types.HostSystemConnectionStateDisconnected) {
		log.Printf("[DEBUG] Desired state is disconnected and host is disconnected")
		return 0, nil
	}

	if !desired && (actual != types.HostSystemConnectionStateDisconnected) {
		log.Printf("[DEBUG] Desired state is disconnected but host is not disconnected. Disconnecting")
		return -1, nil
	}

	log.Printf("[DEBUG] Unexpected combination of desired and actual states, not sure how to handle. Please submit a bug report.")
	return 255, fmt.Errorf("Unexpected combination of connection states")
}

