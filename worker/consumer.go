package worker

import (
	"time"

	log "github.com/opsee/logrus"
	"github.com/nsqio/go-nsq"
	"github.com/opsee/basic/schema"
)

type nsqConsumer struct {
	config      *ConsumerConfig
	consumer    *nsq.Consumer
	eventChan   chan *schema.CheckResult
	stopChan    chan struct{}
	stoppedChan chan struct{}
	logger      *log.Entry
}

type ConsumerConfig struct {
	Topic            string
	Channel          string
	LookupdAddresses []string
	NSQConfig        *nsq.Config
	HandlerCount     int
}

func NewConsumer(config *ConsumerConfig) (*nsqConsumer, error) {
	c := &nsqConsumer{
		config:      config,
		stopChan:    make(chan struct{}, 1),
		stoppedChan: make(chan struct{}, 1),
		eventChan:   make(chan *schema.CheckResult),
		logger:      log.WithField("consumer", "nsq"),
	}

	var err error
	c.consumer, err = nsq.NewConsumer(c.config.Topic, c.config.Channel, c.config.NSQConfig)
	if err != nil {
		log.WithError(err).Error("couldn't create nsq consumer")
		return nil, err
	}

	if c.config.HandlerCount == 0 {
		c.logger.Info("no nsq handler count config detected, setting to 4")
		c.config.HandlerCount = 4
	}

	return c, nil
}

func (c *nsqConsumer) Start() error {
	if c.config.NSQConfig == nil {
		c.logger.Info("no nsq config detected, setting max_in_flight to 4")
		c.config.NSQConfig = nsq.NewConfig()
		c.config.NSQConfig.MaxInFlight = 4
	}

	return c.consumer.ConnectToNSQLookupds(c.config.LookupdAddresses)
}

func (c *nsqConsumer) Stop() {
	c.logger.Info("stopping")
	defer close(c.stopChan)
	defer close(c.stoppedChan)

	c.stopChan <- struct{}{}
	select {
	case <-c.stoppedChan:
	case <-time.After(5 * time.Second):
	}
	c.logger.Info("stopped")
}

func (c *nsqConsumer) AddHandler(handlerFunc func(msg *nsq.Message) error) {
	c.consumer.AddConcurrentHandlers(nsq.HandlerFunc(handlerFunc), c.config.HandlerCount)
}
