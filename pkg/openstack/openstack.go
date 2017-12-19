package openstack

import (
	"errors"
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/networks"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/images"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/sirupsen/logrus"
	"os"
	"regexp"
	"strings"
	"time"
)

func New(nodeup nodeup.NodeUP, key string, keyName string, flavor string) *Openstack {

	o := &Openstack{
		nodeup:     nodeup,
		flavorName: flavor,
		key:        key,
		keyName:    keyName,
	}

	var err error

	opts, err := openstack.AuthOptionsFromEnv()
	o.assertError(err, "AUTH Provide options")

	provider, err := openstack.AuthenticatedClient(opts)
	o.assertError(err, "AUTH Client")

	o.client, err = openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	o.assertError(err, "Compute")

	return o
}

func (o *Openstack) assertError(err error, message string) {
	if err != nil {
		o.Log().Fatalf(message+": %s", err)
	}
}

func (o *Openstack) getFlavorByName() string {
	o.Log().Debugf("Searching FlavorID for Flavor name: %s", o.flavorName)
	flavorID, err := flavors.IDFromName(o.client, o.flavorName)
	o.assertError(err, "Flavor")

	o.Log().Debugf("Found flavor id: %s", flavorID)
	return flavorID
}

func (o *Openstack) getImageByName() string {
	imageID, err := images.IDFromName(o.client, "Ubuntu 16.04-server (64 bit)")
	o.assertError(err, "Error image")

	return imageID
}

func (o *Openstack) getNetworks(defineNetworks string, private bool) ([]string, error) {
	var selectedNetworks []string
	var networksID []string

	allPages, err := networks.List(o.client).AllPages()
	if err != nil {
		o.Log().Errorf("List networks: %s", err)
		return networksID, err
	}
	allNetworks, err := networks.ExtractNetworks(allPages)
	if err != nil {
		o.Log().Errorf("Extract networks: %s", err)
		return networksID, err
	}
	if len(defineNetworks) > 0 {
		selectedNetworks = strings.Split(defineNetworks, ",")
	} else {
		selectedNetworks = append(selectedNetworks, "internet")
	}
	for _, net := range allNetworks {
		for _, selected := range selectedNetworks {
			if regexp.MustCompile(selected).MatchString(net.Label) {
				networksID = append(networksID, net.ID)
			}
		}
	}

	if private {
		for _, net := range allNetworks {
			if net.Label == "local_private" {
				networksID = append(networksID, net.ID)
			}
		}
	}

	return networksID, err
}

func (o *Openstack) createAdminKey() bool {

	//Checking existing keypair
	allPages, err := keypairs.List(o.client).AllPages()
	if err != nil {
		panic(err)
	}

	allKeyPairs, err := keypairs.ExtractKeyPairs(allPages)
	if err != nil {
		panic(err)
	}

	validation := false
	for _, kp := range allKeyPairs {
		if kp.Name == o.keyName {
			o.Log().Debugf("Keypair with name %s already exists", o.keyName)
			o.Log().Debugf("Checking key data for %s", o.keyName)
			if kp.PublicKey == string(o.key) {
				o.Log().Debugf("Keypair with name %s already exists", o.keyName)
				validation = true
			} else {
				o.Log().Debugf("Deleting keypair with name %s", o.keyName)
				err := keypairs.Delete(o.client, o.keyName).ExtractErr()
				if err != nil {
					o.Log().Errorf("Can't delete keypair with name %s", o.keyName)
				}
			}
		}
	}
	if !validation {
		o.Log().Infof("Keypair with name %s does not exist. Creating...", o.keyName)
		keycreateOpts := keypairs.CreateOpts{
			Name:      o.keyName,
			PublicKey: o.key,
		}

		keypair, err := keypairs.Create(o.client, keycreateOpts).Extract()
		if err != nil {
			o.Log().Fatalf("Keypair %s: %s", o.keyName, err)
			return false
		}
		o.Log().Debugf("Keypair %s was created", keypair.Name)
	}

	return true
}

func (o *Openstack) CreateSever(hostname string, networks string, private bool) (*servers.Server, error) {
	if o.isServerExist(hostname) {
		o.Log().Fatalf("Server %s already exists", hostname)
	}

	flavorID := o.getFlavorByName()
	imageID := o.getImageByName()
	networksIDs, err := o.getNetworks(networks, private)
	if err != nil {
		o.Log().Errorf("Error networks: %s", err)
		return nil, err
	}

	o.Log().Infof("Creating server with hostname %s", hostname)

	o.createAdminKey()

	var s []servers.Network

	for _, n := range networksIDs {
		s = append(s, servers.Network{UUID: n})
	}

	serverCreateOpts := servers.CreateOpts{
		Name:      hostname,
		FlavorRef: flavorID,
		ImageRef:  imageID,
		Networks:  s,
	}

	createOpts := keypairs.CreateOptsExt{
		CreateOptsBuilder: serverCreateOpts,
		KeyName:           o.keyName,
	}

	server, err := servers.Create(o.client, createOpts).Extract()
	if err != nil {
		o.Log().Errorf("Error: creating server: %s", err)
		return nil, err
	}

	info := o.getServer(server.ID)

	o.Log().Debugf("Waiting server %s up", info.Name)
	i := 0
	status := ""
	for {
		time.Sleep(5 * time.Second) //Waiting 5 second before retry getting openstack host status
		info = o.getServer(server.ID)

		if info.Status == status {
			i++
			continue
		}

		if info.Status == "ACTIVE" {
			o.Log().Infof("Server %s status is %s", info.Name, info.Status)
			break
		}
		if info.Status == "ERROR" {
			o.Log().Errorf("Bootstrap error: %s", info.Name)
			o.Log().Errorf("Status: %s", info.Status)
			o.Log().Errorf("Fault message: %s", info.Fault.Message)
			o.Log().Errorf("Fault code: %d", info.Fault.Code)
			o.DeleteServer(server.ID)
			return info, errors.New(info.Fault.Message)
		}
		o.Log().Debugf("Server %s status is %s", info.Name, info.Status)
		i++
		if i >= 10 {
			o.Log().Errorf("Timeout for server %s with status %s", info.Name, info.Status)
			o.Log().Errorf("Fault: ", info.Fault.Message)
			return info, errors.New("Timeout")
		}
	}
	return info, nil
}

func (o *Openstack) getServer(sid string) *servers.Server {
	server, _ := servers.Get(o.client, sid).Extract()
	return server
}

func (o *Openstack) isServerExist(name string) bool {
	_, err := servers.IDFromName(o.client, name)
	if err != nil {
		o.Log().Debug(err)
		return false
	} else {
		o.Log().Infof("Server with name %s already exist", name)
		return true
	}
}

func (o *Openstack) Log() *logrus.Entry {
	log := o.nodeup.Log().WithField("context", "openstack")
	return log
}

func (o *Openstack) DeleteServer(sid string) error {
	o.Log().Infof("Deleting server with ID %s", sid)
	result := servers.Delete(o.client, sid)
	if result.Err != nil {
		o.Log().Errorf("Deleting error: %s", result.Err)
	} else {
		o.Log().Infof("Server %s deleted", sid)
	}
	return result.Err
}

func (o *Openstack) DeleteIfError(id string, err error) bool {
	if err != nil {
		o.Log().Error(err)
		err = o.DeleteServer(id)
		if err != nil {
			o.Log().Errorf("Openstack host delete error %s", err)
		}
		return true
	} else {
		return false
	}
}

func (o *Openstack) IDFromName(hostname string) (string, error) {
	id, err := servers.IDFromName(o.client, hostname)
	return id, err
}
