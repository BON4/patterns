package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	yaml "github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

const (
	DEFAULT_MINUTE_RATE          = 100
	DEFAULT_MESSAGE_SIZE         = 100
	DEFAULT_HTTP_TIMEOUT         = 5 * time.Second
	DEFAULT_STATS_PRINT_INTERVAL = time.Second * 10

	WORKER_RESPONSE_SUCCESS = "got ok response"
	WORKER_HTTP_ERR         = "got non ok response"
)

type ConfigRange []int

func (c ConfigRange) From() int {
	if len(c) == 0 {
		return 0
	}

	return c[0]
}

func (c ConfigRange) To() int {
	if len(c) == 0 {
		return 0
	}

	if len(c) == 1 {
		return c[0]
	}

	return c[1]
}

type ClientsConfig struct {
	TargetHost             string        `yaml:"target-host"`
	StatsInterval          time.Duration `yaml:"stats-interval"`
	ClientsNum             int           `yaml:"clients-num"`
	ClientsMinuteRateRange ConfigRange   `yaml:"clients-minute-rate-range,omitempty"`
	ClientsMsgSizeRange    ConfigRange   `yaml:"clients-msg-size-range,omitempty"`
}

func (cfg *ClientsConfig) Validate() error {
	if !strings.HasPrefix(cfg.TargetHost, "http") {
		cfg.TargetHost = "http://" + cfg.TargetHost
	}

	if cfg.TargetHost == "" {
		return errors.New("target-host is required")
	}
	if _, err := url.ParseRequestURI(cfg.TargetHost); err != nil {
		return fmt.Errorf("invalid target-host URL: %w", err)
	}

	if cfg.ClientsNum <= 0 {
		return fmt.Errorf("clients-num must be greater than 0, got %d", cfg.ClientsNum)
	}

	if cfg.StatsInterval < time.Second {
		return fmt.Errorf("stats-interval is too short (min 1s), got %v", cfg.StatsInterval)
	}

	minRate := cfg.ClientsMinuteRateRange.From()
	maxRate := cfg.ClientsMinuteRateRange.To()

	if minRate <= 0 {
		return fmt.Errorf("min minute rate must be positive, got %d", minRate)
	}
	if maxRate < minRate {
		return fmt.Errorf("invalid minute rate range: To (%d) is less than From (%d)", maxRate, minRate)
	}

	if maxRate > 1000000 {
		return fmt.Errorf("minute rate %d is too high (max 1,000,000)", maxRate)
	}

	minSize := cfg.ClientsMsgSizeRange.From()
	maxSize := cfg.ClientsMsgSizeRange.To()

	if minSize < 0 {
		return fmt.Errorf("min message size cannot be negative, got %d", minSize)
	}
	if maxSize < minSize {
		return fmt.Errorf("invalid message size range: To (%d) is less than From (%d)", maxSize, minSize)
	}

	return nil
}

type ClientWorker struct {
	ID         int
	IP         string
	MinuteRate int
	MsgSize    int
	client     *http.Client
	errorCh    chan<- ErrorMessage
	succesCh   chan<- SuccessMessage
}

type ErrorMessage struct {
	WorkerIP string
	Error    error
}

type SuccessMessage struct {
	WorkerIP string
	Response string
}

func (c *ClientWorker) Do(host string) (string, error) {
	req, err := http.NewRequest("GET", host, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("X-Forwarded-For", c.IP)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return "", fmt.Errorf("rate limit exceeded")
		}
		return "", fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	respB, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(respB), nil
}

func (c *ClientWorker) StartForHost(ctx context.Context, host string) {
	interval := time.Duration(60000/c.MinuteRate) * time.Millisecond

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Infof("Started worker: ID:%d, MinRate:%d, MsgSize:%d", c.ID, c.MinuteRate, c.MsgSize)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := c.Do(host)
			if err != nil {
				c.errorCh <- ErrorMessage{
					WorkerIP: c.IP,
					Error:    err,
				}
			} else {
				c.succesCh <- SuccessMessage{
					WorkerIP: c.IP,
					Response: resp,
				}
			}
		}
	}
}

type ClientsManager struct {
	errorStats   sync.Map
	successStats sync.Map
	wg           sync.WaitGroup
	targetHost   string
	clients      []ClientWorker
	errorCh      chan ErrorMessage
	succesCh     chan SuccessMessage
}

func GenerateIP(clientID uint64) string {
	pcg := rand.NewPCG(clientID, 0)
	r := rand.New(pcg)

	ip := net.IPv4(
		byte(r.Uint32N(256)),
		byte(r.Uint32N(256)),
		byte(r.Uint32N(256)),
		byte(r.Uint32N(256)),
	)

	return ip.String()
}

func NewClientsManager(cfg *ClientsConfig) (*ClientsManager, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	cm := ClientsManager{
		clients:    make([]ClientWorker, 0, cfg.ClientsNum),
		errorCh:    make(chan ErrorMessage),
		succesCh:   make(chan SuccessMessage),
		targetHost: cfg.TargetHost,
		wg:         sync.WaitGroup{},
	}

	for i := range cfg.ClientsNum {
		if len(cfg.ClientsMinuteRateRange) == 0 {
			cfg.ClientsMinuteRateRange = []int{DEFAULT_MINUTE_RATE, DEFAULT_MINUTE_RATE}
		}

		if len(cfg.ClientsMsgSizeRange) == 0 {
			cfg.ClientsMsgSizeRange = []int{DEFAULT_MESSAGE_SIZE, DEFAULT_MESSAGE_SIZE}
		}

		httpClient := http.Client{
			Timeout: DEFAULT_HTTP_TIMEOUT,
		}

		cm.clients = append(cm.clients, ClientWorker{
			ID:         i,
			MinuteRate: cfg.ClientsMinuteRateRange.From() + rand.N(cfg.ClientsMinuteRateRange.To()-cfg.ClientsMinuteRateRange.From()+1),
			MsgSize:    cfg.ClientsMsgSizeRange.From() + rand.N(cfg.ClientsMsgSizeRange.To()-cfg.ClientsMsgSizeRange.From()+1),
			errorCh:    cm.errorCh,
			succesCh:   cm.succesCh,
			client:     &httpClient,
			IP:         GenerateIP(uint64(i)),
		})
	}

	return &cm, nil
}

func (cm *ClientsManager) startErrorCollector(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-cm.errorCh:
			v, ok := cm.errorStats.Load(err)
			if !ok {
				cm.errorStats.Store(err, 1)
			} else if count, parsed := v.(int); parsed {
				cm.errorStats.Store(err, count+1)
			}
		}
	}
}

func (cm *ClientsManager) startSuccessCollector(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case success := <-cm.succesCh:
			v, ok := cm.successStats.Load(success)
			if !ok {
				cm.successStats.Store(success, 1)
			} else if count, parsed := v.(int); parsed {
				cm.successStats.Store(success, count+1)
			}
		}
	}
}

type ErrorReport struct {
	ErrorsByWorkers map[string]int
	ErrorsCount     map[string]int
}

func (s ErrorReport) String() string {
	const padding = 3
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, padding, ' ', tabwriter.Debug)

	fmt.Fprintln(&b, "\nERRORS BY WORKERS")
	fmt.Fprintln(w, "Worker IP\tError Count")
	fmt.Fprintln(w, "---------\t-----------")

	workerKeys := make([]string, 0, len(s.ErrorsByWorkers))
	for k := range s.ErrorsByWorkers {
		workerKeys = append(workerKeys, k)
	}
	sort.Strings(workerKeys)

	for _, k := range workerKeys {
		fmt.Fprintf(w, "%s\t%d\n", k, s.ErrorsByWorkers[k])
	}
	w.Flush()

	fmt.Fprintln(&b, "\nERROR TYPES DISTRIBUTION")
	w2 := tabwriter.NewWriter(&b, 0, 0, padding, ' ', tabwriter.Debug)
	fmt.Fprintln(w2, "Error Message\tOccurrences")
	fmt.Fprintln(w2, "-------------\t-----------")

	for msg, count := range s.ErrorsCount {
		fmt.Fprintf(w2, "%s\t%d\n", msg, count)
	}
	w2.Flush()

	return b.String()
}

func (cm *ClientsManager) PullErrors() ErrorReport {
	report := ErrorReport{
		ErrorsByWorkers: make(map[string]int),
		ErrorsCount:     make(map[string]int),
	}

	cm.errorStats.Range(func(key, value any) bool {
		errData, ok := key.(ErrorMessage)
		if !ok {
			return true
		}

		count := value.(int)

		report.ErrorsByWorkers[errData.WorkerIP] += count

		errText := errData.Error.Error()
		report.ErrorsCount[errText] += count

		return true
	})

	return report
}

type SuccessReport struct {
	SuccessByWorkers map[string]int
}

func (s SuccessReport) String() string {
	const padding = 3
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, padding, ' ', tabwriter.Debug)

	fmt.Fprintln(&b, "\nSUCCESS STATS")
	fmt.Fprintln(w, "Worker IP\tSuccess Count")
	fmt.Fprintln(w, "---------\t-------------")

	keys := make([]string, 0, len(s.SuccessByWorkers))
	for k := range s.SuccessByWorkers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%d\n", k, s.SuccessByWorkers[k])
	}
	w.Flush()

	return b.String()
}

func (cm *ClientsManager) PullSuccess() SuccessReport {
	report := SuccessReport{
		SuccessByWorkers: make(map[string]int),
	}

	cm.successStats.Range(func(key, value any) bool {
		success, ok := key.(SuccessMessage)
		if !ok {
			return true
		}

		count, ok := value.(int)
		if !ok {
			return true
		}

		report.SuccessByWorkers[success.WorkerIP] += count

		return true
	})

	return report
}

func (cm *ClientsManager) Start(ctx context.Context) error {
	cm.wg.Go(func() {
		cm.startErrorCollector(ctx)
	})

	cm.wg.Go(func() {
		cm.startSuccessCollector(ctx)
	})

	for _, c := range cm.clients {
		cm.wg.Go(func() {
			c.StartForHost(ctx, cm.targetHost)
		})
	}

	cm.wg.Wait()
	return nil
}

func printStats(ctx context.Context, cm *ClientsManager, interval time.Duration) {
	ticker := time.NewTicker(interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			errorsReport := cm.PullErrors()
			successReport := cm.PullSuccess()

			log.Infof("Stats: %v\n%v\n", successReport.String(), errorsReport.String())
		}
	}
}

func main() {
	var configPath string
	log.SetOutput(os.Stdout)

	pflag.StringVarP(&configPath, "config", "c", "", "Config for clients")
	pflag.Parse()

	if configPath == "" {
		log.Error("config not specified")
		os.Exit(1)
	}

	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	cfg := ClientsConfig{
		ClientsMinuteRateRange: []int{DEFAULT_MINUTE_RATE},
		ClientsMsgSizeRange:    []int{DEFAULT_MESSAGE_SIZE},
		StatsInterval:          DEFAULT_STATS_PRINT_INTERVAL,
	}
	err = yaml.Unmarshal(cfgData, &cfg)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	log.Infof("Used confgig: %+v", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cm, err := NewClientsManager(&cfg)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	go func() {
		cm.Start(ctx)
	}()

	printStats(ctx, cm, cfg.StatsInterval)

	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
}
