package migrate

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

func New(nodeup nodeup.NodeUP) *Migrate {
	m := &Migrate{
		nodeup: nodeup,
	}
	return m
}

func (m *Migrate) Init() {

	m.nodeup.Exitcode = 0

	// handle sigterm correctly
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := m.Log().WithField("signal", s.String())
		logger.Debug("received signal")
		m.nodeup.Stop()
	}()

	m.Log().Infof("NodeUP %s starting", m.nodeup.Ver)
	m.Log().Info("Migration mode enabled")
	m.Log().Infof("Hosts for migration to hypervisor %s: %s", m.nodeup.Hypervisor, m.nodeup.Hosts)

	var wg sync.WaitGroup

	for _, host := range strings.Split(m.nodeup.DeleteWhitespaces(m.nodeup.Hosts), ",") {
		m.Log().Infof("Searching ID for host %s", host)
		hostID, err := m.nodeup.Openstack.IDFromName(host)
		if err != nil {
			m.Log().Fatal(err)
		}
		m.Log().Debugf("HostID for host %s: %s", host, hostID)

		m.Log().Debugf("Starting goroutine for host %s", host)
		wg.Add(1)
		go func(hostID string) {
			if !m.nodeup.Openstack.MigrateHost(hostID, m.nodeup.Hypervisor, &wg) {
				m.nodeup.Exitcode = 1
			}
		}(hostID)
	}
	m.Log().Debug("Waiting for workers to finish")
	wg.Wait()
	os.Exit(m.nodeup.Exitcode)
}
