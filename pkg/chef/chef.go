package chef

import (
	"bytes"
	"encoding/json"
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/go-chef/chef"
	"github.com/sirupsen/logrus"
	"text/template"
)

func New(nodeup nodeup.NodeUP, nodeName string, nodeDomain, chefServerUrl string, validationData []byte, chefValidationPath string, runlist []string) (chef *Chef, err error) {

	chefConfig, err := createConfig(nodeName, ":auto", "STDOUT", chefServerUrl, "chef-validator")
	if err != nil {
		return nil, err
	}

	hosts, err := createHostFile(nodeName, nodeDomain)
	if err != nil {
		return nil, err
	}

	bootstapJson, err := createBootstrapJson(runlist)
	if err != nil {
		return
	}

	chef = &Chef{
		nodeup:        nodeup,
		ChefConfig:    chefConfig,
		BootstrapJson: bootstapJson,
		ValidationPem: validationData,
		Hosts:         hosts,
	}
	return
}

func createConfig(nodeName string, logLevel string, logLocation string, chefServerUrl string, validationClientName string) ([]byte, error) {
	config := &Config{
		LogLevel:             logLevel,
		LogLocation:          logLocation,
		ChefServerUrl:        chefServerUrl,
		ValidationClientName: validationClientName,
		NodeName:             nodeName,
	}

	var buf bytes.Buffer
	t := template.New("client.rb")
	t, err := t.Parse(`
log_level        {{ .LogLevel }}
log_location     {{ .LogLocation }}
chef_server_url  "{{ .ChefServerUrl }}"
validation_client_name "{{ .ValidationClientName }}"
node_name "{{ .NodeName }}"
validation_key "/home/cloud-user/validation.pem"`)
	if err != nil {
		return nil, err
	}
	err = t.Execute(&buf, config) //step 2
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func createHostFile(nodeName string, domainName string) ([]byte, error) {
	hosts := &Hosts{
		Hostname: nodeName,
		Domain:   domainName,
	}

	var buf bytes.Buffer
	t := template.New("hosts")
	t, err := t.Parse(`
127.0.0.1       localhost
127.0.1.1       {{ .Hostname }}.{{ .Domain }}} {{ .Hostname }}`)
	if err != nil {
		return nil, err
	}
	err = t.Execute(&buf, hosts)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func createBootstrapJson(runlist []string) (j []byte, err error) {
	j, err = json.Marshal(Bootstrap{runlist})
	if err != nil {
		return
	}
	return
}

func (c *Chef) Log() *logrus.Entry {
	log := c.Log().WithField("context", "chef")
	return log
}

func NewChefClient(nodeup nodeup.NodeUP, clientName string, key []byte, serverURL string) (*ChefClient, error) {

	client, err := chef.NewClient(&chef.Config{
		Name:    clientName,
		Key:     string(key),
		BaseURL: serverURL,
	})
	if err != nil {
		return nil, err
	}

	c := &ChefClient{
		nodeup: nodeup,
		client: client,
	}

	return c, nil
}

func (c *ChefClient) deleteChefNode(nodeName string) (err error) {
	c.Log().Infof("Deleting chef node %s", nodeName)
	err = c.client.Nodes.Delete(nodeName)
	if err != nil {
		c.Log().Errorf("Delete chef node error: %s", err)
		return
	}
	return
}

func (c *ChefClient) deleteChefClient(clientName string) (err error) {
	c.Log().Infof("Deleting chef client %s", clientName)
	err = c.client.Clients.Delete(clientName)
	if err != nil {
		c.Log().Errorf("Delete chef client error: %s", err)
		return
	}
	return
}

func (c *ChefClient) CleanupNode(nodeName string, clientName string) (status bool, err error) {
	if c.isClientExist(clientName) {
		err = c.deleteChefClient(clientName)
		if err != nil {
			status = false
			return
		}
		err = c.deleteChefNode(clientName)
		if err != nil {
			status = false
			return
		}
	}
	return
}

func (c *ChefClient) isNodeExist(nodeName string) bool {
	if c.isNodeExist(nodeName) {
		_, err := c.client.Nodes.Get(nodeName)
		if err != nil {
			return false
		} else {
			return true
		}
	}
	return true
}

func (c *ChefClient) isClientExist(clientName string) bool {
	_, err := c.client.Clients.Get(clientName)
	if err != nil {
		return false
	} else {
		return true
	}
}

func (c *ChefClient) Log() *logrus.Entry {
	log := c.nodeup.Log().WithField("context", "ssh")
	return log
}
