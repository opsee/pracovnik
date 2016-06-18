package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/sqs"
	etcd "github.com/coreos/etcd/client"
	"github.com/gogo/protobuf/proto"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nsqio/go-nsq"
	"github.com/opsee/basic/schema"
	"github.com/opsee/pracovnik/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

var (
	checkResultsHandled = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "check_results_handled",
		Help: "Total number of check results processed.",
	})
)

func init() {
	prometheus.MustRegister(checkResultsHandled)
}

func main() {
	viper.SetEnvPrefix("pracovnik")
	viper.AutomaticEnv()

	viper.SetDefault("log_level", "info")
	logLevelStr := viper.GetString("log_level")
	logLevel, err := log.ParseLevel(logLevelStr)
	if err != nil {
		log.WithError(err).Error("Could not parse log level, using default.")
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

	go func() {
		hostname, err := os.Hostname()
		if err != nil {
			log.WithError(err).Error("Error getting hostname.")
			return
		}

		ticker := time.Tick(5 * time.Second)
		for {
			<-ticker
			prometheus.Push("pracovnik", hostname, "172.30.35.35:9091")
		}
	}()

	nsqConfig := nsq.NewConfig()
	nsqConfig.MaxInFlight = 4

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	maxTasks := viper.GetInt("max_tasks")
	// in-memory cache of customerId -> bastionId
	bastionMap := map[string]string{}

	consumer, err := worker.NewConsumer(&worker.ConsumerConfig{
		Topic:            "_.results",
		Channel:          "dynamo-results-worker",
		LookupdAddresses: viper.GetStringSlice("nsqlookupd_addrs"),
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

	// TODO(greg): All of the etcd stuff can go once bastions report their
	// bastion id in check results.
	etcdCfg := etcd.Config{
		Endpoints:               []string{viper.GetString("etcd_address")},
		Transport:               etcd.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}
	etcdClient, err := etcd.New(etcdCfg)
	if err != nil {
		log.WithError(err).Fatal("Cannot connect to etcd.")
	}

	kapi := etcd.NewKeysAPI(etcdClient)

	dynamo := &worker.DynamoStore{dynamodb.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))}
	consumer.AddHandler(func(msg *nsq.Message) error {
		result := &schema.CheckResult{}
		if err := proto.Unmarshal(msg.Body, result); err != nil {
			log.WithError(err).Error("Error unmarshalling message from NSQ.")
			return err
		}

		// TODO(greg): Once all bastions have been upgraded to include Bastion ID in
		// their check results, everything in this block can be deleted.
		// -----------------------------------------------------------------------
		if result.Version < 2 {
			// Set bastion ID
			bastionId, ok := bastionMap[result.CustomerId]
			if !ok {
				resp, err := kapi.Get(context.Background(), fmt.Sprintf("/opsee.co/routes/%s", result.CustomerId), nil)
				if err != nil {
					log.WithError(err).Error("Error getting bastion route from etcd.")
					return err
				}

				if len(resp.Node.Nodes) < 1 {
					log.Error("No bastion found for result in etcd.")
					// When we don't find a bastion for this customer, we just drop their results.
					// This isn't a problem after all customers are upgraded.
					return nil
				}

				bastionPath := resp.Node.Nodes[0].Key
				routeParts := strings.Split(bastionPath, "/")
				if len(routeParts) != 5 {
					log.WithError(err).Errorf("Unexpected route length: %d", len(routeParts))
					return err
				}
				bastionId = routeParts[4]
			}
			result.BastionId = bastionId
		}
		// -----------------------------------------------------------------------

		// For now, the region is just static, because we only have dynamodb in one region.
		logger := log.WithFields(log.Fields{
			"customer_id": result.CustomerId,
			"check_id":    result.CheckId,
			"bastion_id":  result.BastionId,
		})

		task := worker.NewCheckWorker(db, dynamo, result)
		_, err = task.Execute()
		if err != nil {
			logger.WithError(err).Error("Error executing task.")
			return err
		}

		checkResultsHandled.Inc()
		return nil
	})

	worker.AddHook(func(id worker.StateId, state *worker.State, result *schema.CheckResult) {
		logger := log.WithFields(log.Fields{
			"customer_id":       state.CustomerId,
			"check_id":          state.CheckId,
			"min_failing_count": state.MinFailingCount,
			"min_failing_time":  state.MinFailingTime,
			"failing_count":     state.FailingCount,
			"failing_time_s":    state.TimeInState().Seconds(),
			"old_state":         state.State,
			"new_state":         id.String(),
		})
		logger.Info("check state changed")
	})

	sqsClient := sqs.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))
	queueUrl := viper.GetString("alerts_sqs_url")

	publishToSQS := func(result *schema.CheckResult) {
		logger := log.WithFields(log.Fields{
			"customer_id": result.CustomerId,
			"check_id":    result.CheckId,
		})

		resultBytes, err := proto.Marshal(result)
		if err != nil {
			logger.WithError(err).Error("Unable to marshal CheckResult to protobuf")
		}
		resultBytesStr := base64.StdEncoding.EncodeToString(resultBytes)
		logger.Infof("Length of message body: %d", len(resultBytesStr))

		if queueUrl == "" {
			logger.Error("No queue URL specified. Not publishing message.")
			return
		}

		_, err = sqsClient.SendMessage(&sqs.SendMessageInput{
			QueueUrl:    aws.String(queueUrl),
			MessageBody: aws.String(resultBytesStr),
		})
		if err != nil {
			logger.WithError(err).Error("Unable to send message to SQS.")
		}
	}

	// TODO(greg): We should be able to set hooks on transitions from->to specific
	// states. Not have to guard in the transition function.
	//
	// transition functions need to be able to signal that we couldn't transition state.
	// in which case we should requeue the message. this could be due to a temporary SQS
	// failure or an error with the result. maybe logging/instrumenting this is enough?
	worker.AddStateHook(worker.StateOK, func(id worker.StateId, state *worker.State, result *schema.CheckResult) {
		logger := log.WithFields(log.Fields{
			"customer_id":       state.CustomerId,
			"check_id":          state.CheckId,
			"min_failing_count": state.MinFailingCount,
			"min_failing_time":  state.MinFailingTime,
			"failing_count":     state.FailingCount,
			"failing_time_s":    state.TimeInState().Seconds(),
			"old_state":         state.State,
			"new_state":         id.String(),
		})

		logger.Infof("check transitioned to passing")
		// We go FAIL -> PASS_WAIT -> OK or WARN
		if state.Id == worker.StatePassWait && id == worker.StateOK {
			publishToSQS(result)
		}
	})

	worker.AddStateHook(worker.StateWarn, func(id worker.StateId, state *worker.State, result *schema.CheckResult) {
		logger := log.WithFields(log.Fields{
			"customer_id":       state.CustomerId,
			"check_id":          state.CheckId,
			"min_failing_count": state.MinFailingCount,
			"min_failing_time":  state.MinFailingTime,
			"failing_count":     state.FailingCount,
			"failing_time_s":    state.TimeInState().Seconds(),
			"old_state":         state.State,
			"new_state":         id.String(),
		})

		logger.Infof("check transitioned to warning")
		// We go FAIL -> PASS_WAIT -> OK or WARN
		if state.Id == worker.StatePassWait && id == worker.StateWarn {
			publishToSQS(result)
		}
	})

	worker.AddStateHook(worker.StateFail, func(id worker.StateId, state *worker.State, result *schema.CheckResult) {
		logger := log.WithFields(log.Fields{
			"customer_id":       state.CustomerId,
			"check_id":          state.CheckId,
			"min_failing_count": state.MinFailingCount,
			"min_failing_time":  state.MinFailingTime,
			"failing_count":     state.FailingCount,
			"failing_time_s":    state.TimeInState().Seconds(),
			"old_state":         state.State,
			"new_state":         id.String(),
		})

		logger.Infof("check transitioned to fail")
		if state.Id == worker.StateFailWait && id == worker.StateFail {
			publishToSQS(result)
		}
	})

	if err := consumer.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start consumer.")
	}

	<-sigChan

	consumer.Stop()
}
