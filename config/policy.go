package config

import (
	"encoding/json"

	"github.com/contiv/errored"
	"github.com/contiv/volplugin/api/schema"
	"github.com/contiv/volplugin/errors"
	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

// Policy is the configuration of the policy. It includes default
// information for items such as pool and volume configuration.
type Policy struct {
	Name           string            `json:"name"`
	Unlocked       bool              `json:"unlocked,omitempty" merge:"unlocked"`
	CreateOptions  CreateOptions     `json:"create"`
	RuntimeOptions RuntimeOptions    `json:"runtime"`
	DriverOptions  map[string]string `json:"driver"`
	FileSystems    map[string]string `json:"filesystems"`
	Backends       BackendDrivers    `json:"backends"`
}

// BackendDrivers is a struct containing all the drivers used under this policy
type BackendDrivers struct {
	CRUD     string `json:"crud"`
	Mount    string `json:"mount"`
	Snapshot string `json:"snapshot"`
}

// NewPolicy return policy config with specified backend preset
func NewPolicy() *Policy {
	return &Policy{}
}

var defaultFilesystems = map[string]string{
	"ext4": "mkfs.ext4 -m0 %",
}

const defaultFilesystem = "ext4"

func (c *Client) policy(name string) string {
	return c.prefixed(rootPolicy, name)
}

// PublishPolicy publishes policy intent to the configuration store.
func (c *Client) PublishPolicy(name string, cfg *Policy) error {
	cfg.Name = name

	value, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	// create the volume directory for the policy so that files can be written there.
	// for example: /volplugin/policies/policy1 will create
	// /volplugin/volumes/policy1 so that a volume of policy1/test can be created
	// at /volplugin/volumes/policy1/test
	c.etcdClient.Set(context.Background(), c.prefixed(rootVolume, name), "", &client.SetOptions{Dir: true})

	if _, err := c.etcdClient.Set(context.Background(), c.policy(name), string(value), &client.SetOptions{PrevExist: client.PrevIgnore}); err != nil {
		return errors.EtcdToErrored(err)
	}

	return nil
}

// DeletePolicy removes a policy from the configuration store.
func (c *Client) DeletePolicy(name string) error {
	_, err := c.etcdClient.Delete(context.Background(), c.policy(name), nil)
	return errors.EtcdToErrored(err)
}

// GetPolicy retrieves a policy from the configuration store.
func (c *Client) GetPolicy(name string) (*Policy, error) {
	if name == "" {
		return nil, errored.Errorf("Policy invalid: empty string for name")
	}

	resp, err := c.etcdClient.Get(context.Background(), c.policy(name), nil)
	if err != nil {
		return nil, errors.EtcdToErrored(err)
	}

	if err := schema.ValidatePolicy([]byte(resp.Node.Value)); err != nil {
		return nil, err
	}

	tc := NewPolicy()
	if err := json.Unmarshal([]byte(resp.Node.Value), tc); err != nil {
		return nil, err
	}

	tc.Name = name

	return tc, nil
}

// ListPolicies provides an array of strings corresponding to the name of each
// policy.
func (c *Client) ListPolicies() ([]Policy, error) {
	resp, err := c.etcdClient.Get(context.Background(), c.prefixed(rootPolicy), &client.GetOptions{Recursive: true, Sort: true})
	if err != nil {
		return nil, errors.EtcdToErrored(err)
	}

	policies := []Policy{}
	for _, node := range resp.Node.Nodes {
		policy := Policy{}
		if err := json.Unmarshal([]byte(node.Value), &policy); err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

func (cfg *Policy) String() string {
	return cfg.Name
}
