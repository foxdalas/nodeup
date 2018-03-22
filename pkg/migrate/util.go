package migrate

import (
	"github.com/sirupsen/logrus"
)

func (m *Migrate) Log() *logrus.Entry {
	log := m.nodeup.Log().WithField("context", "migrate")
	return log
}
