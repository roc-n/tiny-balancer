package balancer

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// penalty for the host with no load
const (
	penalty   = 1000000
	decayTime = int64(time.Second * 10)
)

func init() {
	factories[P2C_EWMABalancer] = NewP2C_EWMA
}

type hostEntity struct {
	name     string
	inflight int64
	lag      uint64
	last     int64
}

// P2C_EWMA refer to the power of 2 random choice using EWMA
type P2C_EWMA struct {
	sync.RWMutex
	hosts   []*hostEntity
	rnd     *rand.Rand
	loadMap map[string]*hostEntity
}

func NewP2C_EWMA(hosts []string) Balancer {
	p := &P2C_EWMA{
		hosts:   []*hostEntity{},
		loadMap: make(map[string]*hostEntity),
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	for _, h := range hosts {
		p.Add(h)
	}
	return p
}

// Add new host to the balancer
func (p *P2C_EWMA) Add(hostName string) {
	p.Lock()
	defer p.Unlock()

	if _, ok := p.loadMap[hostName]; ok {
		return
	}

	h := &hostEntity{name: hostName}
	p.hosts = append(p.hosts, h)
	p.loadMap[hostName] = h
}

// Remove host from the balancer
func (p *P2C_EWMA) Remove(host string) {
	p.Lock()
	defer p.Unlock()

	if _, ok := p.loadMap[host]; !ok {
		return
	}

	delete(p.loadMap, host)
	for i, h := range p.hosts {
		if h.name == host {
			p.hosts = append(p.hosts[:i], p.hosts[i+1:]...)
			return
		}
	}
}

// Balance selects a suitable host according to the key value
func (p *P2C_EWMA) Balance(key string) (string, error) {
	p.RLock()
	defer p.RUnlock()

	var chosen *hostEntity
	switch len(p.hosts) {
	case 0:
		return "", NoHostError
	case 1:
		chosen = p.choose(p.hosts[0], nil)
	default:
		a := p.rnd.Intn(len(p.hosts))
		b := p.rnd.Intn(len(p.hosts) - 1)
		if b >= a {
			b++
		}
		node1 := p.hosts[a]
		node2 := p.hosts[b]

		chosen = p.choose(node1, node2)
	}

	return chosen.name, nil
}

func (p *P2C_EWMA) choose(h1, h2 *hostEntity) *hostEntity {
	if h2 == nil {
		return h1
	}

	if h1.load() > h2.load() {
		// TODO 强制选择很久没选的
		h1, h2 = h2, h1
	}

	return h1
}

func (h *hostEntity) load() int64 {
	lag := int64(math.Sqrt(float64(atomic.LoadUint64(&h.lag) + 1)))
	load := lag * (atomic.LoadInt64(&h.inflight) + 1)
	if load == 0 {
		return penalty
	}
	return load
}

// Inc refers to the number of connections to the server `+1`
func (p *P2C_EWMA) Inc(host string) {
	p.Lock()
	defer p.Unlock()

	h, ok := p.loadMap[host]

	if !ok {
		return
	}

	atomic.AddInt64(&h.inflight, 1)
}

// Done refers to the number of connections to the server `-1`
func (p *P2C_EWMA) Done(host string) {
	p.Lock()
	defer p.Unlock()

	h, ok := p.loadMap[host]

	if !ok {
		return
	}
	atomic.AddInt64(&h.inflight, -1)
}

// RequestCtx refers to the transaction of the request context
func (p *P2C_EWMA) RequestCtx() func(string) {
	start := time.Now().UnixNano()
	return func(host string) {
		p.Lock()
		defer p.Unlock()

		h, _ := p.loadMap[host]

		now := time.Now().UnixNano()

		// update last access time
		last := atomic.SwapInt64(&h.last, now)
		td := now - last
		if td < 0 {
			td = 0
		}
		w := math.Exp(float64(-td) / float64(decayTime))

		lag := now - start
		if lag < 0 {
			lag = 0
		}
		olag := atomic.LoadUint64(&h.lag)
		if olag == 0 {
			w = 0
		}

		// The smaller the value of w, the lower the impact of historical data.
		atomic.StoreUint64(&h.lag, uint64(float64(olag)*w+float64(lag)*(1-w)))
	}
}
