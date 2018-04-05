package rebalance

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"github.com/sirupsen/logrus"
)

type Rebalance struct {
	nodeup nodeup.NodeUP
	log    *logrus.Entry
}

type hypervisorsStatistics struct {
	hypervisors []h
	vmcount     int
}

type HSorted []h

type h struct {
	name  string
	vms   []string
	count int
}

type migrateToItem struct {
	hypervisorName string
	count          int
}

type migrateTo struct {
	migration []migrateToItem
	count     int
}
