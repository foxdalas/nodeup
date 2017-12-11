package nodeup

import (
	log "github.com/sirupsen/logrus"

	"errors"
	"flag"
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

	"io/ioutil"
	"net"
	"os/exec"
	"os/user"
	"time"
)

var _ nodeup.NodeUP = &NodeUP{}

func New(version string) *NodeUP {
	return &NodeUP{
		version:   version,
		log:       makeLog(),
		stopCh:    make(chan struct{}),
		waitGroup: sync.WaitGroup{},
	}
}

func (o *NodeUP) Init() {
	o.Log().Infof("NodeUP %s starting", o.version)

	o.exitcode = 0

	// handle sigterm correctly
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := o.Log().WithField("signal", s.String())
		logger.Debug("received signal")
		o.Stop()
	}()

	// parse env vars
	err := o.params()
	if err != nil {
		o.Log().Fatal(err)
	}

	if _, err := os.Stat(o.logDir); os.IsNotExist(err) {
		o.Log().Debugf("Creating logs directory in %s", o.logDir)
		os.Mkdir(o.logDir, 0775)
	}

	if o.count > 1 && !o.isWildcard(o.name) {
		o.Log().Panicf("Can't create more one host with not unique name. Please set -count 1")
	}

	s := openstack.New(o, o.osPublicKey, o.osKeyName, o.osFlavorName)
	chefClient, err := chef.NewChefClient(o, o.chefClientName, o.chefKeyPem, o.chefServerUrl)
	if err != nil {
		o.Log().Fatalf("Chef client error: %s", err)
	}

	var wg sync.WaitGroup
	for _, hostname := range o.nameGenerator(o.name, o.count) {
		o.Log().Debugf("Starting goroutine for host %s", hostname)
		wg.Add(1)
		go func(hostname string) {

			if !o.bootstrapHost(s, chefClient, hostname, &wg) {
				o.exitcode = 1
			}
		}(hostname)
	}
	o.Log().Debug("Waiting for workers to finish")
	wg.Wait()

	os.Exit(o.exitcode)
}

func (o *NodeUP) bootstrapHost(s *openstack.Openstack, c *chef.ChefClient, hostname string, wg *sync.WaitGroup) bool {
	defer wg.Done()
	if o.jenkinsMode {
		o.Log().Infof("Processing log %s%s.log", o.jenkinsLogURL, hostname)
	}

	oHost, err := s.CreateSever(hostname, o.defineNetworks)
	if err != nil {
		return false
	}

	logFile := o.logDir + "/" + hostname + ".log"
	outFile, err := os.Create(logFile)
	if err != nil {
		return false
	}

	var availableAddresses []string
	for _, ip := range o.getPublicAddress(oHost.Addresses) {

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
		chefData, err := chef.New(o, hostname, o.domain, o.chefServerUrl, o.chefValidationPem, o.chefValidationPath, []string{"role[" + o.chefRole + "]"})
		if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
			return false
		}

		o.Log().Infof("Bootstrapping host %s", hostname)
		//Upload files via ssh
		for fileName, fileData := range o.transferFiles(chefData) {
			err = sshClient.TransferFile(fileData, fileName, o.sshUploadDir)
			if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
				return false
			}
		}

		//Run command via ssh
		for _, command := range o.runCommands(o.sshUploadDir, o.chefVersion, o.chefEnvironment) {
			err = sshClient.RunCommandPipe(command, outFile)
			if o.assertBootstrap(s, c, oHost.ID, hostname, err) {
				return false
			}
		}
	}
	return true
}

func makeLog() *log.Entry {
	logtype := strings.ToLower(os.Getenv("LOG_TYPE"))
	if logtype == "" {
		logtype = "text"
	}
	if logtype == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else if logtype == "text" {
		log.SetFormatter(&log.TextFormatter{
			ForceColors: true,
		})
	} else {
		log.WithField("logtype", logtype).Fatal("Given logtype was not valid, check LOG_TYPE configuration")
		os.Exit(1)
	}

	loglevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if len(loglevel) == 0 {
		log.SetLevel(log.InfoLevel)
	} else if loglevel == "debug" {
		log.SetLevel(log.DebugLevel)
	} else if loglevel == "info" {
		log.SetLevel(log.InfoLevel)
	} else if loglevel == "warn" {
		log.SetLevel(log.WarnLevel)
	} else if loglevel == "error" {
		log.SetLevel(log.ErrorLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	return log.WithField("context", "nodeup")
}

func (o *NodeUP) params() error {

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&o.name, "name", "", "Hostname or  mask like role-environment-* or full-hostname-name if -count 1")
	flag.StringVar(&o.domain, "domain", "", "Domain name like hosts.example.com")
	flag.StringVar(&o.logDir, "logDir", "logs", "Logs directory")
	flag.IntVar(&o.count, "count", 1, "Deployment hosts count")
	flag.StringVar(&o.osFlavorName, "flavor", "", "Openstack flavor name")
	flag.StringVar(&o.chefEnvironment, "chefEnvironment", "", "Environment name for host")
	flag.StringVar(&o.chefRole, "chefRole", "", "Role name for host")
	flag.StringVar(&o.osKeyName, "keyName", usr.Username, "Openstack admin key name")
	flag.StringVar(&o.osPublicKeyPath, "publicKeyPath", "", "Openstack admin key path")
	flag.StringVar(&o.user, "user", "cloud-user", "Openstack user")
	flag.BoolVar(&o.ignoreFail, "ignoreFail", false, "Don't delete host after fail")
	flag.IntVar(&o.concurrency, "concurrency", 5, "Concurrency bootstrap")
	flag.IntVar(&o.prefixCharts, "prefixCharts", 5, "Host mask random prefix")
	flag.IntVar(&o.sshWaitRetry, "sshWaitRetry", 10, "SSH Retry count")
	flag.StringVar(&o.chefVersion, "chefVersion", "12.20.3", "chef-client version")
	flag.StringVar(&o.chefServerUrl, "chefServerUrl", "", "Chef Server URL")
	flag.StringVar(&o.chefClientName, "chefClientName", "", "Chef client name")
	flag.StringVar(&o.chefKeyPath, "chefKeyPath", "", "Chef client certificate path")
	flag.StringVar(&o.chefValidationPath, "chefValidationPath", "", "Validation key path or CHEF_VALIDATION_PEM")
	flag.StringVar(&o.sshUser, "sshUser", "cloud-user", "SSH Username")
	flag.StringVar(&o.sshUploadDir, "sshUploadDir", "/home/"+o.sshUser, "SSH Upload directory")
	flag.StringVar(&o.defineNetworks, "networks", "", "Define networks like 8.8.8.0/24, 10.0.0.0/24")

	flag.BoolVar(&o.jenkinsMode, "jenkinsMode", false, "Jenkins capability mode")

	flag.Parse()

	if o.chefValidationPath == "" && len(os.Getenv("CHEF_VALIDATION_PEM")) == 0 {
		return errors.New("Please provide -chefValidationPath or environment variable CHEF_VALIDATION_PEM")
	} else {
		if len(os.Getenv("CHEF_VALIDATION_PEM")) > 0 {
			o.chefValidationPem = []byte(os.Getenv("CHEF_VALIDATION_PEM"))
		} else {
			o.chefValidationPem, err = ioutil.ReadFile(o.chefValidationPath)
			if err != nil {
				o.Log().Errorf("Chef validation read error: %s", err)
			}
		}
	}

	if o.chefKeyPath == "" && len(os.Getenv("CHEF_KEY_PEM")) == 0 {
		return errors.New("Please provide -chefKeyPath or environment variable CHEF_KEY_PEM")
	} else {
		if len(os.Getenv("CHEF_KEY_PEM")) > 0 {
			o.chefKeyPem = []byte(os.Getenv("CHEF_KEY_PEM"))
		} else {
			o.chefKeyPem, err = ioutil.ReadFile(o.chefKeyPath)
			if err != nil {
				o.Log().Errorf("Chef key read error: %s", err)
			}
		}
	}

	if o.chefServerUrl == "" && len(os.Getenv("CHEF_SERVER_URL")) == 0 {
		return errors.New("Please provide -chefServerUrl or environment variable CHEF_SERVER_URL")
	} else {
		if len(os.Getenv("CHEF_SERVER_URL")) > 0 {
			o.chefServerUrl = os.Getenv("CHEF_SERVER_URL")
		}
	}

	if o.chefClientName == "" && len(os.Getenv("CHEF_CLIENT_NAME")) == 0 {
		return errors.New("Please provide -chefClientName or environment variable CHEF_CLIENT_NAME")
	} else {
		if len(os.Getenv("CHEF_CLIENT_NAME")) > 0 {
			o.chefClientName = os.Getenv("CHEF_CLIENT_NAME")
		}
	}

	if o.chefRole == "" {
		return errors.New("Please provide -chefRole string")
	}

	if o.chefEnvironment == "" {
		return errors.New("Please provide -chefEnvironment string")
	}

	if o.name == "" {
		return errors.New("Please provide -name string")
	}

	if o.domain == "" {
		return errors.New("Please provide -domain string")
	}

	if o.count == 0 {
		return errors.New("Please provide -count int")
	}

	if o.osPublicKeyPath == "" && len(os.Getenv("OS_PUBLIC_KEY")) == 0 {
		return errors.New("Please provide -keyPath or environment variable OS_PUBLIC_KEY")
	} else {
		if o.osPublicKeyPath != "" {
			dat, err := ioutil.ReadFile(o.osPublicKeyPath)
			if err != nil {
				log.Fatal(err)
			}
			o.osPublicKey = string(dat)
		} else {
			o.osPublicKey = os.Getenv("OS_PUBLIC_KEY")
		}
	}

	if o.osFlavorName == "" {
		return errors.New("Please provide -flavor string")
	}

	if o.osKeyName == "" {
		return errors.New("Please provide -keyname string")
	}

	o.osAuthURL = os.Getenv("OS_AUTH_URL")
	if len(o.osAuthURL) == 0 {
		return errors.New("Please provide OS_AUTH_URL")
	}

	o.osTenantName = os.Getenv("OS_TENANT_NAME")
	if len(o.osTenantName) == 0 {
		return errors.New("Please provide OS_TENANT_NAME")
	}

	o.osUsername = os.Getenv("OS_USERNAME")
	if len(o.osUsername) == 0 {
		return errors.New("Please provide OS_USERNAME")
	}

	o.osPassword = os.Getenv("OS_PASSWORD")
	if len(o.osPassword) == 0 {
		return errors.New("Please provide OS_PASSWORD")
	}

	o.osPassword = os.Getenv("OS_PASSWORD")
	if len(o.osPassword) == 0 {
		return errors.New("Please provide OS_PASSWORD")
	}

	if o.jenkinsMode {
		o.jenkinsLogURL = os.Getenv("JOB_URL") + "ws/logs/"
	}

	return nil
}

func (o *NodeUP) Stop() {
	o.Log().Info("shutting things down")
	close(o.stopCh)
	os.Exit(0)
}

func (o *NodeUP) Log() *log.Entry {
	return o.log
}

func (o *NodeUP) Version() string {
	return o.version
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

func (o *NodeUP) getPublicAddress(addresses map[string]interface{}) []string {
	var result []string

	//TODO: Please fix this shit
	for _, networks := range addresses {
		for _, addrs := range networks.([]interface{}) {
			ip := addrs.(map[string]interface{})["addr"].(string)
			result = append(result, ip)
		}
	}
	return result
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
	for i := 0; i < o.sshWaitRetry; i++ {
		err := sshConnect(address)
		if err != nil {
			o.Log().Warnf("Cannot connect to host %s #%d: %s", address, i+1, err.Error())
		} else {
			return true
		}
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
	if o.ignoreFail {
		o.Log().Warnf("Host %s bootstrap is fail. Skied")
		return false
	}

	if err != nil {
		o.Log().Errorf("Bootstrap error: %s", err)
		host := openstack.DeleteIfError(id, err)
		chefClient, err := chefClient.CleanupNode(hostname, hostname)
		if err != nil {
			o.Log().Errorf("Chef cleanup node error %s", err)
			o.exitcode = 1
			return true
		}

		if !host && !chefClient {
			o.Log().Errorf("Can't cleanup node %s", hostname)
			o.exitcode = 1
			return true
		}
		o.exitcode = 1
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
		"sudo chef-client -c " + dir + "/client.rb -E" + environment + " -j " + dir + "/bootstrap.json",
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
