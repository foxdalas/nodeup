package chef

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateConfig(t *testing.T) {
	r, err := createConfig("test-node", ":auto", "STDOUT", "http://localhost", "chef-validator")
	assert.Equal(t, nil, err)
	testData := `
log_level        :auto
log_location     STDOUT
chef_server_url  "http://localhost"
validation_client_name "chef-validator"
node_name "test-node"
validation_key "/home/cloud-user/validation.pem"`
	fmt.Print(string(r))
	assert.Equal(t, testData, string(r))
}

func TestCreateHostFile(t *testing.T) {
	r, err := createHostFile("test", "hostname.example.com")
	assert.Equal(t, nil, err)

	testData := `
127.0.0.1       localhost
127.0.1.1       test.hostname.example.com test`
	assert.Equal(t, testData, string(r))
}

func TestCreateBootstrapJson(t *testing.T) {
	r, err := createBootstrapJson([]string{"role[test]"})
	assert.Equal(t, nil, err)
	testData := `{"run_list":["role[test]"]}`
	assert.Equal(t, testData, string(r))
}
