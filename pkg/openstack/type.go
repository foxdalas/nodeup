package openstack

import (
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/sirupsen/logrus"
	"github.com/gophercloud/gophercloud"

)

type Openstack struct {
	nodeup nodeup.NodeUP
	client *gophercloud.ServiceClient
	flavorName string
	key string
	keyName string

	log *logrus.Entry
}

