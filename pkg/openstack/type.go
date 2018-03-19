package openstack

import (
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/gophercloud/gophercloud"
	"github.com/sirupsen/logrus"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/hypervisors"
)

type Openstack struct {
	nodeup     nodeup.NodeUP
	client     *gophercloud.ServiceClient
	flavorName string
	key        string
	keyName    string

	log *logrus.Entry
}

type sortedHypervisorsByvCPU []hypervisors.Hypervisor
type sortedHypervisorsBMemory []hypervisors.Hypervisor
