package migrate

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"github.com/sirupsen/logrus"
)

type Migrate struct {
	nodeup nodeup.NodeUP
	log    *logrus.Entry
}
