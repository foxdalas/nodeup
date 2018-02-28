package nodeup

import (
	"github.com/foxdalas/nodeup/pkg/chef"
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/foxdalas/nodeup/pkg/openstack"
	"github.com/foxdalas/nodeup/pkg/ssh"
	garbler "github.com/michaelbironneau/garbler/lib"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	"os/exec"
	"text/template"
	"time"
)

var _ nodeup.NodeUP = &NodeUP{}

func New(version string, logging *log.Entry) *NodeUP {
	return &NodeUP{
		Ver:       version,
		Logging:   logging,
		StopCh:    make(chan struct{}),
		WaitGroup: sync.WaitGroup{},
	}
}

func (o *NodeUP) Init() {
	o.Log().Infof("NodeUP %s starting", o.Ver)

	o.Exitcode = 0

	// handle sigterm correctly
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := o.Log().WithField("signal", s.String())
		logger.Debug("received signal")
		o.Stop()
	}()

	if _, err := os.Stat(o.LogDir); os.IsNotExist(err) {
		o.Log().Debugf("Creating logs directory in %s", o.LogDir)
		os.Mkdir(o.LogDir, 0775)
	}

	if o.Count > 1 && !o.isWildcard(o.Name) {
		o.Log().Panicf("Can't create more one host with not unique name. Please set -count 1")
	}

	if o.DeleteNodes != "" {
		exit := 0
		for _, hostname := range strings.Split(o.DeleteNodes, ",") {
			serverID, err := o.Openstack.IDFromName(hostname)
			if err != nil {
				o.Log().Errorf("Can't retrive serverID: %s", err)
			}
			err = o.Openstack.DeleteServer(serverID)
			if err != nil {
				o.Log().Errorf("Server %s delete problem openstack", hostname)
				exit = 1
			} else {
				o.Log().Infof("Server %s successfully deleted from openstack", hostname)
			}
			_, err = o.Chef.CleanupNode(hostname, hostname)
			if err != nil {
				o.Log().Errorf("Server %s delete problem chef", hostname)
				o.Log().Error(err)
				exit = 1
			} else {
				o.Log().Infof("Server %s successfully deleted from chef", hostname)
			}
		}
		os.Exit(exit)
	}

	var wg sync.WaitGroup
	for _, hostname := range o.nameGenerator(o.Name, o.Count) {
		o.Log().Debugf("Starting goroutine for host %s", hostname)
		wg.Add(1)
		go func(hostname string) {
			if !o.bootstrapHost(o.Openstack, o.Chef, hostname, &wg) {
				o.Exitcode = 1
			}
		}(hostname)
	}
	o.Log().Debug("Waiting for workers to finish")
	wg.Wait()
	os.Exit(o.Exitcode)
}

func (o *NodeUP) bootstrapHost(s *openstack.Openstack, c *chef.ChefClient, hostname string, wg *sync.WaitGroup) bool {
	defer wg.Done()
	if o.JenkinsMode {
		o.Log().Infof("Processing log %s%s.log", o.JenkinsLogURL, hostname)
	}

	oHost, err := s.CreateSever(hostname, o.OSGroupID, o.DefineNetworks)
	if err != nil {
		return false
	}

	logFile := o.LogDir + "/" + hostname + ".log"
	outFile, err := os.Create(logFile)
	if err != nil {
		return false
	}

	var availableAddresses []string

	ipAddresses := o.GetAddress(oHost.Addresses)
	o.Log().Debugf("Ip Addresses for host %s: %s", hostname, strings.Join(ipAddresses, ","))
	for _, ip := range ipAddresses {

		if o.checkSSHPort(ip) {
			o.Log().Debugf("SSH is accessible on host %s", hostname)
			availableAddresses = append(availableAddresses, ip)
		} else {
			o.Log().Errorf("SSH is unreachable on host %s", hostname)
			s.DeleteServer(oHost.ID)
			return false
		}

		if len(availableAddresses) == 0 {
			o.Log().Errorf("Can't bootstrap host %s no SSH access", hostname)
			s.DeleteServer(oHost.ID)
			return false
		}

		//Create SSH connection
		sshClient, err := ssh.New(o, ip, "cloud-user")
		if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
			return false
		}

		//Create Bootstrap data
		chefData, err := chef.New(o, hostname, o.Domain, o.ChefServerUrl, o.ChefValidationPem, o.ChefValidationPath, []string{"role[" + o.ChefRole + "]"})
		if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
			return false
		}

		o.Log().Infof("Bootstrapping host %s", hostname)
		//Upload files via ssh
		for fileName, fileData := range o.transferFiles(chefData) {
			err = sshClient.TransferFile(fileData, fileName, o.SSHUploadDir)
			if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
				return false
			}
		}

		if o.UsePrivateNetwork {
			err = sshClient.TransferFile(o.createInterfacesFile(o.Gateway), "interfaces", o.SSHUploadDir)
			if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
				return false
			}
			for _, command := range o.configureDefaultGateway() {
				err = sshClient.RunCommandPipe(command, outFile)
				if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
					return false
				}
			}
		}

		//Run command via ssh
		for _, command := range o.runCommands(o.SSHUploadDir, o.ChefVersion, o.ChefEnvironment) {
			err = sshClient.RunCommandPipe(command, outFile)
			if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
				return false
			}
		}
	}
	return true
}

func (o *NodeUP) Stop() {
	o.Log().Info("shutting things down")
	close(o.StopCh)
	os.Exit(0)
}

func (o *NodeUP) Log() *log.Entry {
	return o.Logging
}

func (o *NodeUP) Version() string {
	return o.Ver
}

func (o *NodeUP) nameGenerator(prefix string, count int) []string {

	o.Log().Debugf("Generation hostname for %d hosts", count)

	var result []string

	req := garbler.PasswordStrengthRequirements{
		MinimumTotalLength: 5,
		MaximumTotalLength: 5,
		Uppercase:          0,
		Digits:             2,
		Punctuation:        0,
	}

	//Infinity loop for find duplicates
	i := 0
	for {
		if i == count {
			break
		}
		s, err := garbler.NewPassword(&req)
		if err != nil {
			o.Log().Errorf("Can't generate prefix for hostname: %s", err)
		}
		if !contains(result, s) {
			result = append(result, strings.Replace(prefix, "*", s, -1))
			i++
		}
	}
	return result
}

func (o *NodeUP) GetAddress(addresses map[string]interface{}) []string {
	var public []string
	var private []string

	for _, networks := range addresses {
		for _, addrs := range networks.([]interface{}) {
			ip := addrs.(map[string]interface{})["addr"].(string)

			if o.publicIP(ip) {
				o.Log().Debugf("IP %s is public", ip)
				public = append(public, ip)
			}
			if o.privateIP(ip) {
				o.Log().Debugf("IP %s is private", ip)
				private = append(private, ip)
			}
		}
	}

	if len(public) > 0 {
		o.Log().Debugf("Found public ip's: %s", public)
		return public
	} else {
		o.UsePrivateNetwork = true
		return private
	}
}

func sshConnect(address string) error {
	conn, err := net.DialTimeout("tcp", address+":22", 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	return err
}

func (o *NodeUP) checkSSHPort(address string) bool {
	o.Log().Infof("Waiting SSH on host %s", address)
	time.Sleep(10 * time.Second) //Waiting ssh daemon
	for i := 0; i <= o.SSHWaitRetry; i++ {
		err := sshConnect(address)
		if err != nil {
			o.Log().Warnf("Cannot connect to host %s #%d: %s", address, i+1, err.Error())
		} else {
			return true
		}
		o.Log().Infof("Retries left %d", o.SSHWaitRetry-i)
		time.Sleep(10 * time.Second)
	}
	o.Log().Errorf("Can't connect to host %s via ssh", address)

	return false
}

func (o *NodeUP) deleteChefNode(hostname string) {
	cmdName := "knife"
	cmdArgs := []string{"node", "delete", hostname, "-y"}

	if _, err := exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		o.Log().Errorf("Can't delete chef client %s: %s", hostname, err)
	} else {
		o.Log().Infof("Chef node %s deleted", hostname)
	}

	cmdArgs = []string{"client", "delete", hostname, "-y"}
	if _, err := exec.Command(cmdName, cmdArgs...).Output(); err == nil {
		o.Log().Infof("Chef client %s deleted", hostname)
	} else {
		o.Log().Errorf("Can't delete chef node %s: %s", hostname, err)
	}
}

func (o *NodeUP) isWildcard(string string) bool {
	if strings.ContainsAny(string, "*") {
		return true
	} else {
		return false
	}
}

func (o *NodeUP) assertBootstrap(openstack *openstack.Openstack, chefClient *chef.ChefClient, id string, hostname string, err error) (exit bool) {
	if o.IgnoreFail {
		o.Log().Warnf("Host %s bootstrap is fail. Skied", hostname)
		return false
	}

	if err != nil {
		o.Log().Errorf("Bootstrap error: %s", err)
		host := openstack.DeleteIfError(id, err)
		chefClient, err := chefClient.CleanupNode(hostname, hostname)
		if err != nil {
			o.Log().Errorf("Chef cleanup node error %s", err)
			o.Exitcode = 1
			return true
		}

		if !host && !chefClient {
			o.Log().Errorf("Can't cleanup node %s", hostname)
			o.Exitcode = 1
			return true
		}
		o.Exitcode = 1
		return true
	}
	return false
}

func (o *NodeUP) deleteHost(openstack *openstack.Openstack, chefClient *chef.ChefClient, id string, hostname string) {
	err := openstack.DeleteServer(id)
	if err != nil {
		o.Log().Errorf("Openstack delete server error %s", err)
	}
	_, err = chefClient.CleanupNode(hostname, hostname)
	if err != nil {
		o.Log().Errorf("Chef cleanup node error %s", err)
	}
}

func (o *NodeUP) configureDefaultGateway() []string {
	data := []string{
		"sudo mv interfaces /etc/network/",
		fmt.Sprintf("sudo route add default gw %s", o.Gateway),
	}
	return data
}

func (o *NodeUP) transferFiles(chef *chef.Chef) map[string][]byte {
	data := make(map[string][]byte)
	data["bootstrap.json"] = chef.BootstrapJson
	data["validation.pem"] = chef.ValidationPem
	data["client.rb"] = chef.ChefConfig
	data["hosts"] = chef.Hosts

	return data
}

func (o *NodeUP) runCommands(dir string, version string, environment string) []string {
	data := []string{
		"sudo mv hosts /etc/hosts && sudo hostname -F /etc/hostname",
		"sudo apt-get update",
		"sudo mkdir /etc/chef",
		"wget -q https://omnitruck.chef.io/install.sh && sudo bash ./install.sh -v " + version + " && rm install.sh",
		"sudo chmod 0600 " + dir + "/validation.pem",
		"sudo chef-client -c " + dir + "/client.rb -E " + environment + " -j " + dir + "/bootstrap.json",
		"sudo rm " + dir + "/client.rb && sudo rm " + dir + "/validation.pem && rm " + dir + "/bootstrap.json",
		"sudo chef-client",
	}
	return data
}

func contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}

func (o *NodeUP) privateIP(ip string) bool {
	private := false
	IP := net.ParseIP(ip)
	if IP == nil {
		o.Log().Error("Invalid IP")
	} else {
		//_, private24BitBlock, _ := net.ParseCIDR("10.0.0.0/8") // this block for Global Private Network.
		_, private20BitBlock, _ := net.ParseCIDR("172.16.0.0/12")
		_, private16BitBlock, _ := net.ParseCIDR("192.168.0.0/16")
		private = private20BitBlock.Contains(IP) || private16BitBlock.Contains(IP)
	}

	return private
}

func (o *NodeUP) publicIP(ip string) bool {
	IP := net.ParseIP(ip)
	if IP.IsLoopback() || IP.IsLinkLocalMulticast() || IP.IsLinkLocalUnicast() {
		return false
	}
	if ip4 := IP.To4(); ip4 != nil {
		switch true {
		case ip4[0] == 10:
			return false
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return false
		case ip4[0] == 192 && ip4[1] == 168:
			return false
		default:
			return true
		}
	}
	return false
}

func (o *NodeUP) createInterfacesFile(gateway string) []byte {
	interfaces := &Interfaces{
		Gateway: gateway,
	}

	var buf bytes.Buffer
	t := template.New("interfaces")
	t, err := t.Parse(`
auto lo
iface lo inet loopback

allow-hotplug ens3
iface ens3 inet dhcp
  post-up route add default gw {{ .Gateway }}

allow-hotplug ens4
iface ens4 inet dhcp

allow-hotplug ens5
iface ens5 inet dhcp

source /etc/network/interfaces.d/*`)
	if err != nil {
		o.Log().Fatal(err)
	}
	err = t.Execute(&buf, interfaces)
	if err != nil {
		o.Log().Fatal(err)
	}
	return buf.Bytes()
}
