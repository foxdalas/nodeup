package chef

import "github.com/sirupsen/logrus"

func (c *ChefClient) Log() *logrus.Entry {
	log := c.nodeup.Log().WithField("context", "ssh")
	return log
}
