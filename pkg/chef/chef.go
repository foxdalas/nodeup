package chef

import (
	"bytes"
	"encoding/json"
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/go-chef/chef"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"text/template"
)

func New(nodeup nodeup.NodeUP, nodeName string, chefServerUrl string, validationData []byte, runlist []string) (chef *Chef, err error) {

	chefConfig, err := createConfig(nodeName, ":auto", "STDOUT", chefServerUrl, "chef-validator")
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
	t, err := template.ParseFiles("templates/config.tmpl")
	if err != nil {
		return nil, err
	}
	err = t.Execute(&buf, config) //step 2
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

func NewChefClient(nodeup nodeup.NodeUP, clientName string, keyPath string, serverURL string) (*ChefClient, error) {
	key, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

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
	return
}

func (c *ChefClient) Log() *logrus.Entry {
	log := c.nodeup.Log().WithField("context", "ssh")
	return log
}
