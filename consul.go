package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bilibili/kratos/pkg/log"
	"github.com/bilibili/kratos/pkg/naming"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	llog "log"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ERR_INS_ADDRS_EMPTY = errors.New("len of ins.Addrs should not be 0")
)

// Config discovery configures.
type Config struct {
	Nodes  []string
	Region string
	Zone   string
	Env    string
	Host   string
}

// Resolver resolve naming service
type Resolver struct {
	appID   string
	c       chan struct{}
	client  *api.Client
	agent   *api.Agent
	plan    *watch.Plan
	builder *Builder
	node    atomic.Value
}

func (resolver Resolver) watch() error {
	var params map[string]interface{}
	watchKey := fmt.Sprintf(`{"type":"service", "service":"%s"}`, resolver.appID)
	if err := json.Unmarshal([]byte(watchKey), &params); err != nil {
		return err
	}
	plan, err := watch.Parse(params)
	if err != nil {
		return err
	}
	plan.Handler = func(idx uint64, raw interface{}) {
		log.Info("watch notify %v", raw)
		resolver.c <- struct{}{}
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(map[string][]string)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if v["consul"] == nil {
			log.Error(`v["consul"] == nil`)
			return
		}
		log.Info("watch notify %v", v)
		//resolver.c <- struct{}{}
	}
	logger := llog.New(os.Stdout, "", llog.LstdFlags) // @todo replace logger
	go func() {
		err := plan.RunWithClientAndLogger(resolver.client, logger)
		if err != nil {
			log.Error("watch service %s error %s", resolver.appID, err.Error())
		}
	}()
	resolver.plan = plan
	return nil
}

func (resolver Resolver) Watch() <-chan struct{} {
	return resolver.c
}

func (resolver Resolver) fetch(c context.Context) (*naming.InstancesInfo, bool) {
	_, infoArr, err := resolver.agent.AgentHealthServiceByName(resolver.appID)
	if err != nil {
		log.Error("get AgentHealthServiceByName %s err %s", resolver.appID, err.Error())
		return nil, false
	}
	instancesInfo := &naming.InstancesInfo{}
	instancesInfo.Scheduler = make([]naming.Zone, 0, 10)
	instancesInfo.Instances = make(map[string][]*naming.Instance)
	log.Info("get AgentHealthServiceByName %s info len %d", resolver.appID, len(infoArr))
	for _, info := range infoArr {
		log.Info("get AgentHealthServiceByName %s info addr %s:%d", resolver.appID, info.Service.Address, info.Service.Port)
		ins := resolver.coverService2Instance(info.Service)
		if _, ok := instancesInfo.Instances[ins.Zone]; !ok {
			instancesInfo.Instances[ins.Zone] = make([]*naming.Instance, 0, 10)
		}
		instancesInfo.Instances[ins.Zone] = append(instancesInfo.Instances[ins.Zone], ins)
	}
	instancesInfo.LastTs = time.Now().Unix()
	return instancesInfo, true
}
func (resolver Resolver) Fetch(c context.Context) (ins *naming.InstancesInfo, ok bool) {
	ins, ok = resolver.node.Load().(*naming.InstancesInfo)
	return
}

func (resolver Resolver) Close() error {
	if !resolver.plan.IsStopped() {
		resolver.plan.Stop()
	}
	return nil
}

func (resolver Resolver) coverService2Instance(service *api.AgentService) *naming.Instance {
	meta := service.Meta
	addr := []string{
		service.Address + ":" + strconv.Itoa(service.Port),
	}
	ins := &naming.Instance{
		Region:   meta["region"],
		Zone:     meta["zone"],
		Env:      meta["env"],
		Hostname: meta["hostname"],
		Version:  meta["version"],
		AppID:    service.Service,
		Addrs:    addr,
	}
	delete(meta, "region")
	delete(meta, "zone")
	delete(meta, "env")
	delete(meta, "hostname")
	delete(meta, "version")
	ins.Metadata = meta
	return ins
}

func (builder Builder) coverIns2AgentService(ins *naming.Instance) ([]*api.AgentServiceRegistration, error) {
	if len(ins.Addrs) == 0 {
		return nil, ERR_INS_ADDRS_EMPTY
	}
	registrationArr := make([]*api.AgentServiceRegistration, len(ins.Addrs))
	meta := make(map[string]string)
	meta["region"] = ins.Region
	meta["zone"] = ins.Zone
	meta["env"] = ins.Env
	meta["hostname"] = ins.Hostname
	meta["version"] = ins.Version
	meta["last_ts"] = strconv.FormatInt(ins.LastTs, 10)

	for key, value := range ins.Metadata {
		meta[key] = value
	}
	for i, addr := range ins.Addrs {
		urlVal, err := url.Parse(addr)
		if err != nil {
			return nil, err
		}
		port, _ := strconv.Atoi(urlVal.Port())
		service := &api.AgentServiceRegistration{
			ID:      ins.AppID + "-" + urlVal.Hostname() + "-" + urlVal.Port(),
			Name:    ins.AppID,
			Kind:    api.ServiceKindTypical,
			Port:    port,
			Address: urlVal.Scheme + "://" + urlVal.Hostname(),
			Meta:    meta,
		}
		registrationArr[i] = service
	}
	return registrationArr, nil
}

func (resolver *Resolver) selfproc() {
	for {
		_, ok := <-resolver.c
		if !ok {
			return
		}
		zones, ok := resolver.fetch(context.Background())
		if ok {
			resolver.newSelf(zones.Instances)
		}
	}
}

func (resolver *Resolver) newSelf(zones map[string][]*naming.Instance) {
	ins, ok := zones[resolver.builder.c.Zone]
	if !ok {
		return
	}
	var nodes []string
	for _, in := range ins {
		for _, addr := range in.Addrs {
			u, err := url.Parse(addr)
			if err == nil && u.Scheme == "http" {
				nodes = append(nodes, u.Host)
			}
		}
	}
	// diff old nodes
	var olds int
	for _, n := range nodes {
		if node, ok := resolver.node.Load().([]string); ok {
			for _, o := range node {
				if o == n {
					olds++
					break
				}
			}
		}
	}
	if len(nodes) == olds {
		return
	}
	rand.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})
	resolver.node.Store(nodes)
}

func (builder Builder) Register(ctx context.Context, ins *naming.Instance) (cancel context.CancelFunc, err error) {
	serviceArr, err := builder.coverIns2AgentService(ins)
	if err != nil {
		return
	}

	ctx, cancel = context.WithCancel(ctx)
	for _, service := range serviceArr { //@todo 批量注册
		service.Check = &api.AgentServiceCheck{
			TTL:    "15s",
			Status: api.HealthPassing,
		}
		var status string
		var info *api.AgentServiceChecksInfo
		status, info, err = builder.agent.AgentHealthServiceByID(service.ID)
		if err != nil {
			return
		}
		if info == nil && status == api.HealthCritical {
			err = builder.agent.ServiceRegister(service) // @todo check had registered
			if err != nil {
				return
			}
		} else {
			err = builder.agent.PassTTL(fmt.Sprintf("service:%s", service.ID), "I am good :)")
			if err != nil {
				return
			}
		}

		go func(service *api.AgentServiceRegistration) {
			for {
				select {
				case <-ctx.Done():
					err := builder.agent.ServiceDeregister(service.ID)
					if err != nil {
						log.Error("consul: ServiceDeregister %s err: %s", service.ID, err.Error())
					}
					return
				case <-time.After(time.Second * 5):
					err := builder.agent.PassTTL(fmt.Sprintf("service:%s", service.ID), "I am good :)")
					if err != nil {
						log.Error("consul: ServiceRegister %s err: %s", service.ID, err.Error())
					}
				}
			}
		}(service)
	}
	return
}

func (builder Builder) Close() error {
	return nil
}

type Builder struct {
	client *api.Client
	agent  *api.Agent
	r      map[string]*Resolver
	locker sync.RWMutex
	c      *Config
}

func (builder Builder) Build(id string) naming.Resolver {
	builder.locker.RLock()
	if r, ok := builder.r[id]; ok {
		builder.locker.RUnlock()
		return r
	}
	builder.locker.RUnlock()
	builder.locker.Lock()
	r := &Resolver{
		appID:   id,
		client:  builder.client,
		agent:   builder.agent,
		builder: &builder,
	}
	r.c = make(chan struct{}, 10)
	builder.r[id] = r
	builder.locker.Unlock()
	err := r.watch()
	if err != nil {
		log.Error("watch error %s", err.Error())
	}
	r.c <- struct {}{}
	go r.selfproc()
	return r
}

func (builder Builder) Scheme() string {
	return "consul"
}

func NewConsulDiscovery(c Config) (builder Builder, err error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return
	}
	builder.client = client
	builder.agent = client.Agent()
	builder.r = make(map[string]*Resolver)
	builder.c = &c
	return
}
