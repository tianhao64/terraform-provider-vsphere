package vsphere

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-vsphere/vsphere/internal/helper/hostsystem"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"log"
	"strings"
)

func resourceVsphereNic() *schema.Resource {
	return &schema.Resource{
		Create: resourceVsphereNicCreate,
		Read:   resourceVsphereNicRead,
		Update: resourceVsphereNicUpdate,
		Delete: resourceVsphereNicDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: vmKernelSchema(),
	}
}

func vmKernelSchema() map[string]*schema.Schema {
	base := BaseVMKernelSchema()
	base["host"] = &schema.Schema{
		Type:        schema.TypeString,
		Required:    true,
		Description: "ESX host the interface belongs to",
	}

	return base
}

func resourceVsphereNicRead(d *schema.ResourceData, meta interface{}) error {
	ctx := context.TODO()
	client := meta.(*VSphereClient).vimClient
	tfNicID := d.Id()

	toks := strings.Split(tfNicID, "_")
	hostID := toks[0]
	nicID := toks[1]

	vnic, err := getVnicFromHost(ctx, client, hostID, nicID)
	if err != nil {
		log.Printf("[DEBUG] Nic (%s) not found. Probably deleted.", nicID)
		d.SetId("")
		return nil
	}

	log.Printf("[DEBUG] %t", *vnic.Spec.Ip.IpV6Config.DhcpV6Enabled)
	log.Printf("[DEBUG] %t", *vnic.Spec.Ip.IpV6Config.AutoConfigurationEnabled)
	d.Set("portgroup", vnic.Portgroup)
	d.Set("distributed_switch_port", vnic.Spec.DistributedVirtualPort.SwitchUuid)
	d.Set("distributed_port_group", vnic.Spec.DistributedVirtualPort.PortgroupKey)
	d.Set("mtu", vnic.Spec.Mtu)
	d.Set("mac", vnic.Spec.Mac)
	d.Set("ipv4.0.dhcp", vnic.Spec.Ip.Dhcp)
	d.Set("ipv4.0.ip", vnic.Spec.Ip.IpAddress)
	d.Set("ipv4.0.netmask", vnic.Spec.Ip.SubnetMask)
	d.Set("ipv6.0.dhcp", *vnic.Spec.Ip.IpV6Config.DhcpV6Enabled)
	d.Set("ipv6.0.autoconfig", *vnic.Spec.Ip.IpV6Config.AutoConfigurationEnabled)
	dhcp, ok := d.GetOk("ipv6.0.dhcp")
	log.Printf("[DEBUG] %t - %t - %t", *vnic.Spec.Ip.IpV6Config.DhcpV6Enabled, dhcp.(bool), ok)
	log.Printf("[DEBUG] %t - %t", *vnic.Spec.Ip.IpV6Config.AutoConfigurationEnabled, d.Get("ipv6.0.autoconfig").(bool))

	return nil
}

func resourceVsphereNicCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	hostID := d.Get("host").(string)
	ctx := context.TODO()

	portgroup := d.Get("portgroup").(string)
	nic, err := getNicSpecFromSchema(d)
	if err != nil {
		return err
	}

	host, err := hostsystem.FromID(client, hostID)
	if err != nil {
		return err
	}
	cmRef := host.ConfigManager().Reference()
	cm := object.NewHostConfigManager(client.Client, cmRef)
	hns, err := cm.NetworkSystem(ctx)
	if err != nil {
		log.Printf("[DEBUG] Failed to access the host's NetworkSystem service: %s", err)
		return err
	}

	nicID, err := hns.AddVirtualNic(ctx, portgroup, *nic)
	if err != nil {
		if soap.IsSoapFault(err) {
			sf := soap.ToSoapFault(err)
			log.Printf("[DEBUG] SOAP Error while creating Nic: %#v", sf)

		}
		return err
	}
	log.Printf("[DEBUG] Created NIC with ID: %s", nicID)
	tfNicID := fmt.Sprintf("%s_%s", hostID, nicID)
	d.SetId(tfNicID)

	vnic, err := getVnicFromHost(ctx, client, hostID, nicID)
	if err != nil {
		log.Printf("Error while retrieving vNic(%s) info post-creation", nicID)
	}
	d.Set("mac", vnic.Spec.Mac)
	d.Set("mtu", vnic.Spec.Mtu)

	return resourceVsphereNicRead(d, meta)
}

func resourceVsphereNicUpdate(d *schema.ResourceData, meta interface{}) error {
	_ = meta.(*VSphereClient).vimClient

	return resourceVsphereNicRead(d, meta)
}

func resourceVsphereNicDelete(d *schema.ResourceData, meta interface{}) error {
	_ = meta.(*VSphereClient).vimClient
	return nil
}

// VmKernelSchema returns the schema required to represent a vNIC adapter on an ESX Host.
// We make this public so we can pull this from the host resource as well.
func BaseVMKernelSchema() map[string]*schema.Schema {
	sch := map[string]*schema.Schema{
		"portgroup": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "portgroup to attach the nic to. Do not set if you set distributed_switch_port.",
		},
		"distributed_switch_port": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "UUID of the DVSwitch the nic will be attached to. Do not set if you set portgroup.",
		},
		"distributed_port_group": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Key of the distributed portgroup the nic will connect to",
		},
		"ipv4": {
			Type:     schema.TypeList,
			Optional: true,
			MaxItems: 1,
			Elem: &schema.Resource{Schema: map[string]*schema.Schema{
				"dhcp": {
					Type:        schema.TypeBool,
					Optional:    true,
					Description: "Use DHCP to configure the interface's IPv4 stack.",
					Default:     false,
				},
				"ip": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "address of the interface, if DHCP is not set.",
					Default:     "",
				},
				"netmask": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "netmask of the interface, if DHCP is not set.",
					Default:     "",
				},
				"gw": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "IP address of the default gateway, if DHCP is not set.",
					Default:     "",
				},
			}},
		},
		"ipv6": {
			Type:     schema.TypeList,
			Optional: true,
			MaxItems: 1,
			Elem: &schema.Resource{Schema: map[string]*schema.Schema{
				"dhcp": {
					Type:        schema.TypeBool,
					Optional:    true,
					Description: "Use DHCP to configure the interface's IPv4 stack.",
					Default:     false,
				},
				"autoconfig": {
					Type:        schema.TypeBool,
					Optional:    true,
					Description: "Use IPv6 Autoconfiguration (RFC2462).",
					Default:     false,
				},
				"addresses": {
					Type:        schema.TypeList,
					Optional:    true,
					Description: "List of IPv6 addresses",
					Elem: &schema.Schema{
						Type: schema.TypeString,
					},
				},
				"gw": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "IP address of the default gateway, if DHCP or autoconfig is not set.",
					Default:     "",
				},
			}},
		},
		"mac": {
			Type:        schema.TypeString,
			Optional:    true,
			Computed:    true,
			Description: "MAC address of the interface.",
		},
		"mtu": {
			Type:        schema.TypeInt,
			Optional:    true,
			Computed:    true,
			Description: "MTU of the interface.",
		},
	}
	return sch
}

func getNicSpecFromSchema(d *schema.ResourceData) (*types.HostVirtualNicSpec, error) {
	portgroup := d.Get("portgroup").(string)
	dvp := d.Get("distributed_switch_port").(string)
	dpg := d.Get("distributed_port_group").(string)
	mac := d.Get("mac").(string)
	mtu := int32(d.Get("mtu").(int))
	ipv4 := d.Get("ipv4").([]interface{})
	ipv6 := d.Get("ipv6").([]interface{})

	if portgroup != "" && dvp != "" {
		return nil, fmt.Errorf("portgroup and distributed_switch_port settings are mutually exclusive.")
	}

	var dvpPortConnection *types.DistributedVirtualSwitchPortConnection
	if portgroup != "" {
		dvpPortConnection = nil
	} else {
		dvpPortConnection = &types.DistributedVirtualSwitchPortConnection{
			SwitchUuid:   dvp,
			PortgroupKey: dpg,
		}
	}

	ipConfig := &types.HostIpConfig{}
	if len(ipv4) > 0 {
		ipv4Config := ipv4[0].(map[string]interface{})
		dhcp := ipv4Config["dhcp"].(bool)
		ipv4Address := ipv4Config["ip"].(string)
		ipv4Netmask := ipv4Config["netmask"].(string)
		if dhcp && ipv4Address != "" {
			return nil, fmt.Errorf("ip and dhcp are mutually exclusive")
		}
		ipConfig.Dhcp = dhcp
		if ipv4Address != "" && ipv4Netmask != "" {
			ipConfig.IpAddress = ipv4Address
			ipConfig.SubnetMask = ipv4Netmask
		}
	}

	if len(ipv6) > 0 {
		ipv6Spec := &types.HostIpConfigIpV6AddressConfiguration{}
		ipv6Config := ipv6[0].(map[string]interface{})
		dhcpv6 := ipv6Config["dhcp"].(bool)
		autoconfig := ipv6Config["autoconfig"].(bool)
		ipv6addrs := ipv6Config["addresses"].([]interface{})
		if dhcpv6 {
			if autoconfig || len(ipv6addrs) > 0 {
				return nil, fmt.Errorf("DHCP is set to true. You neither set autoconfig to true nor pass a list of addresses.")
			}
			ipv6Spec.DhcpV6Enabled = &dhcpv6
		} else if autoconfig {
			if dhcpv6 || len(ipv6addrs) > 0 {
				return nil, fmt.Errorf("Autoconfig is set to true. You neither set dhcp to true nor pass a list of addresses.")
			}
			ipv6Spec.AutoConfigurationEnabled = &autoconfig
		}
		ipConfig.IpV6Config = ipv6Spec
	}

	// TODO: Routes
	vnic := &types.HostVirtualNicSpec{
		Ip:                     ipConfig,
		Mac:                    mac,
		Mtu:                    mtu,
		Portgroup:              portgroup,
		DistributedVirtualPort: dvpPortConnection,
	}
	log.Printf("[DEBUG] About to send Nic Spec: %#v", vnic)

	return vnic, nil

}

func getVnicFromHost(ctx context.Context, client *govmomi.Client, hostID, nicID string) (*types.HostVirtualNic, error) {
	host, err := hostsystem.FromID(client, hostID)
	if err != nil {
		return nil, err
	}

	var hostProps mo.HostSystem
	err = host.Properties(ctx, host.Reference(), nil, &hostProps)
	if err != nil {
		log.Printf("[DEBUG] Failed to get the host's properties: %s", err)
		return nil, err
	}
	vNics := hostProps.Config.Network.Vnic
	nicIdx := -1
	for idx, vnic := range vNics {
		log.Printf("[DEBUG] Evaluating nic: %#v", vnic.Device)
		if vnic.Device == nicID {
			nicIdx = idx
			break
		}
	}

	if nicIdx == -1 {
		return nil, fmt.Errorf("VMKernel interface with id %s not found", nicID)
	}
	return &vNics[nicIdx], nil
}
