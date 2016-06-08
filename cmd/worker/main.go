package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nsqio/go-nsq"
	"github.com/opsee/basic/schema"
	"github.com/opsee/pracovnik/worker"
	"github.com/spf13/viper"
)

func main() {
	viper.SetEnvPrefix("pracovnik")
	viper.AutomaticEnv()

	nsqConfig := nsq.NewConfig()
	nsqConfig.MaxInFlight = 4

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	maxTasks := viper.GetInt("max_tasks")

	consumer, err := worker.NewConsumer(&worker.ConsumerConfig{
		Topic:            "_.results",
		Channel:          "dynamo-results-worker",
		LookupdAddresses: viper.GetStringSlice("lookupd_addresses"),
		NSQConfig:        nsqConfig,
		HandlerCount:     maxTasks,
	})

	if err != nil {
		log.WithError(err).Fatal("Failed to create consumer.")
	}

	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	if err != nil {
		log.WithError(err).Fatal("Cannot connect to database.")
	}

	consumer.AddHandler(func(msg *nsq.Message) error {
		result := &schema.CheckResult{}
		if err := proto.Unmarshal(msg.Body, result); err != nil {
			log.WithError(err).Error("Error unmarshalling message from NSQ.")
			return err
		}

		task := worker.NewCheckWorker(db, result)
		_, err = task.Execute()
		if err != nil {
			return err
		}

		return nil
	})

	worker.AddHook(worker.StateOK, func(id worker.StateId, state *worker.State) {
		logger := log.WithFields(log.Fields{
			"customer_id":       state.CustomerId,
			"check_id":          state.CheckId,
			"min_failing_count": state.MinFailingCount,
			"min_failing_time":  state.MinFailingTime,
			"failing_count":     state.NumFailing,
			"failing_time_s":    state.TimeInState().Seconds(),
		})
		logger.Infof("check moving from state %s -> %s", state.State, worker.StateStrings[id])
	})

	if err := consumer.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start consumer.")
	}

	<-sigChan

	consumer.Stop()
}
