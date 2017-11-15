package nodeup

import "github.com/sirupsen/logrus"

type NodeUP interface {

	Version() string
	Log() *logrus.Entry
}

type Openstack interface {
	CreateServer()
}