package consul

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/lubanproj/gorpc/plugin"
	"github.com/lubanproj/gorpc/selector"
	"net/http"
	"strings"
)

type Consul struct {
	opts *plugin.Options
	client *api.Client
	config *api.Config
	balancerName string  // load balancing mode, including random, polling, weighted polling, consistent hash, etc
	writeOptions *api.WriteOptions
	queryOptions *api.QueryOptions
}

const Name = "consul"

func init() {
	plugin.Register(Name, ConsulSvr)
	selector.RegisterSelector(Name, ConsulSvr)
}

var ConsulSvr = &Consul {
	opts : &plugin.Options{},
}

func (c *Consul) InitConfig() error {

	config := api.DefaultConfig()
	c.config = config

	config.HttpClient = http.DefaultClient
	config.Address = c.opts.SelectorSvrAddr
	config.Scheme = "http"

	client, err := api.NewClient(config)
	if err != nil {
		return err
	}

	c.client = client

	return nil
}


func (c *Consul) Resolve(serviceName string) ([]*selector.Node, error) {

	pairs, _, err := c.client.KV().List(serviceName, nil)
	if err != nil {
		return nil, err
	}

	if len(pairs) == 0 {
		return nil, fmt.Errorf("no services find in path : %s", serviceName)
	}
	var nodes []*selector.Node
	for _, pair := range pairs {
		nodes = append(nodes, &selector.Node {
			Key : pair.Key,
			Value : pair.Value,
		})
	}
	return nodes, nil
}

// 实现 selector 服务发现
func (c *Consul) Select(serviceName string) (string, error) {

	nodes, err := c.Resolve(serviceName)

	if nodes == nil || len(nodes) == 0 || err != nil {
		return "", err
	}

	balancer := selector.GetBalancer(c.balancerName)
	node := balancer.Balance(nodes)

	if node == nil {
		return "", fmt.Errorf("no services find in %s", serviceName)
	}

	return parseAddrFromNode(node)
}

func parseAddrFromNode(node *selector.Node) (string, error){
	if node.Key == "" {
		return "", errors.New("addr is empty")
	}

	strs := strings.Split(node.Key, "/")

	return strs[len(strs)-1], nil
}

func (c *Consul) Init(opts ...plugin.Option) error {

	for _, o := range opts {
		o(c.opts)
	}

	if len(c.opts.Services) == 0 || c.opts.SvrAddr == "" || c.opts.SelectorSvrAddr == "" {
		return fmt.Errorf("consul init error, len(services) : %d, svrAddr : %s, selectorSvrAddr : %s",
			len(c.opts.Services), c.opts.SvrAddr, c.opts.SelectorSvrAddr)
	}

	if err := c.InitConfig(); err != nil {
		return err
	}

	for _, serviceName := range c.opts.Services {
		nodeName := fmt.Sprintf("%s/%s", serviceName, c.opts.SvrAddr)

		kvPair := &api.KVPair{
			Key : nodeName,
			Value : []byte(c.opts.SvrAddr),
			Flags: api.LockFlagValue,
		}

		if _, err := c.client.KV().Put(kvPair, c.writeOptions); err != nil {
			return err
		}
	}


	return nil
}

func Init(consulSvrAddr string, opts ... plugin.Option) error {
	for _, o := range opts {
		o(ConsulSvr.opts)
	}

	ConsulSvr.opts.SelectorSvrAddr = consulSvrAddr
	err := ConsulSvr.InitConfig()
	return err
}
