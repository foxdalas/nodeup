package openstack

import "github.com/sirupsen/logrus"

func (o *Openstack) Log() *logrus.Entry {
	log := o.nodeup.Log().WithField("context", "openstack")
	return log
}

func (o *Openstack) assertError(err error, message string) {
	if err != nil {
		o.Log().Fatalf(message+": %s", err)
	}
}

func (c sortedHypervisorsByvCPU) Len() int           { return len(c) }
func (c sortedHypervisorsByvCPU) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c sortedHypervisorsByvCPU) Less(i, j int) bool { return c[i].VCPUsUsed > c[j].VCPUsUsed }

func (c sortedHypervisorsBMemory) Len() int           { return len(c) }
func (c sortedHypervisorsBMemory) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c sortedHypervisorsBMemory) Less(i, j int) bool { return c[i].FreeRamMB > c[j].FreeRamMB }
