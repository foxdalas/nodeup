package rebalance

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"github.com/foxdalas/nodeup/pkg/openstack"
	"math"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
)

func New(nodeup nodeup.NodeUP) *Rebalance {
	r := &Rebalance{
		nodeup: nodeup,
	}
	return r
}

func (r *Rebalance) Init() {

	r.nodeup.Exitcode = 0

	// handle sigterm correctly
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := r.Log().WithField("signal", s.String())
		logger.Debug("received signal")
		r.nodeup.Stop()
	}()

	r.Log().Infof("NodeUP %s starting", r.nodeup.Ver)
	r.Log().Info("Rebalance mode enabled")
	r.Log().Info("Processing server list")

	allServers, err := r.nodeup.Openstack.GetServers()
	if err != nil {
		r.Log().Fatal(err)
	}

	var ids []string
	var servers []openstack.Server

	for _, host := range allServers {
		if strings.Contains(host.Name, r.nodeup.Hosts) {
			ids = append(ids, host.ID)
		}
	}

	for _, id := range ids {
		server, err := r.nodeup.Openstack.GetServerDetail(id)
		if err != nil {
			r.Log().Fatal(err)
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		os.Exit(0)
	}
	migrationPlan := r.findMigration(r.calculateUsage(servers))
	r.rebalance(migrationPlan)
}

func (r *Rebalance) calculateUsage(servers []openstack.Server) (*hypervisorsStatistics, map[string][]string) {

	vmByHypervisor := make(map[string][]string)

	hUsage := &h{}
	hStatistics := &hypervisorsStatistics{}

	for _, server := range servers {
		vmByHypervisor[server.HypervisorName] = append(vmByHypervisor[server.HypervisorName], server.Name)
		hStatistics.vmcount += 1
	}

	for name, vms := range vmByHypervisor {
		hUsage.name = name
		hUsage.vms = vms
		hUsage.count = len(vms)
		hStatistics.hypervisors = append(hStatistics.hypervisors, *hUsage)
	}

	for _, h := range hStatistics.hypervisors {
		h.count = len(h.vms)
	}

	sort.Sort(HSorted(hStatistics.hypervisors))

	return hStatistics, vmByHypervisor
}

func (r *Rebalance) calculateBestWedth(status *hypervisorsStatistics) int {
	count := float64(status.vmcount) / float64(len(status.hypervisors))
	if float64(int(count)) == count {
		return int(count)
	}

	return int(count) + 1
}

func (r *Rebalance) calculateMigration(status *hypervisorsStatistics) (map[string]int, *migrateTo) {
	fromHypervisor := make(map[string]int)
	migrateToItem := &migrateToItem{}
	migrateTo := &migrateTo{}

	perHS := r.calculateBestWedth(status)

	for _, hypervisor := range status.hypervisors {
		r.Log().Infof("Hypervisor %s vm count %d", hypervisor.name, hypervisor.count)

		hsState := perHS - hypervisor.count
		if hsState == 0 {
			continue
		}
		if hsState < 0 {
			fromHypervisor[hypervisor.name] = int(math.Abs(float64(hsState)))
		} else {
			migrateToItem.hypervisorName = hypervisor.name
			migrateToItem.count = hsState
			migrateTo.count += migrateToItem.count
			migrateTo.migration = append(migrateTo.migration, *migrateToItem)
		}
	}

	return fromHypervisor, migrateTo
}

func (r *Rebalance) findMigration(status *hypervisorsStatistics, vmByHypervisor map[string][]string) map[string]string {
	var migrateVMList []string
	migratePlan := make(map[string]string)

	fromHypervisor, migrateTo := r.calculateMigration(status)

	for hypervisorName, count := range fromHypervisor {
		migrateVMList = append(migrateVMList, vmByHypervisor[hypervisorName][0:count]...)
	}

	if len(migrateVMList) == 0 {
		r.Log().Info("Nothing to migrate")
		os.Exit(0)
	}

	j := 0

outer:
	for _, toHypervisor := range migrateTo.migration {
		for i := 0; i < toHypervisor.count; i++ {
			if j+1 > len(migrateVMList) {
				break outer
			}
			r.Log().Infof("Migration plan add vm %s to %s", migrateVMList[j], toHypervisor.hypervisorName)
			migratePlan[migrateVMList[j]] = toHypervisor.hypervisorName
			j += 1
		}
	}

	return migratePlan
}

func (r *Rebalance) rebalance(migrationPlan map[string]string) {
	var wg sync.WaitGroup

	for hostname, hypervisorName := range migrationPlan {
		r.Log().Infof("Searching ID for host %s", hostname)
		hostID, err := r.nodeup.Openstack.IDFromName(hostname)
		if err != nil {
			r.Log().Fatal(err)
		}
		r.Log().Infof("HostID for host %s: %s", hostname, hostID)

		r.Log().Debugf("Starting goroutine for host %s", hostname)
		wg.Add(1)
		if !r.nodeup.Openstack.MigrateHost(hostID, hypervisorName, &wg) {
			r.nodeup.Exitcode = 1
		}

	}
	r.Log().Debug("Waiting for workers to finish")
	wg.Wait()
	os.Exit(r.nodeup.Exitcode)
}
