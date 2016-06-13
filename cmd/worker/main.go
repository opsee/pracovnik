package main

import (
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
	etcd "github.com/coreos/etcd/client"
	"github.com/golang/protobuf/proto"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nsqio/go-nsq"
	"github.com/opsee/basic/schema"
	"github.com/opsee/pracovnik/worker"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

func main() {
	viper.SetEnvPrefix("pracovnik")
	viper.AutomaticEnv()

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
					log.WithError(err).Error("No bastion found for result in etcd.")
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
		dynamo := &worker.DynamoStore{dynamodb.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))}
		task := worker.NewCheckWorker(db, dynamo, result)
		_, err = task.Execute()
		if err != nil {
			return err
		}

		return nil
	})

	worker.AddHook(func(id worker.StateId, state *worker.State) {
		logger := log.WithFields(log.Fields{
			"customer_id":       state.CustomerId,
			"check_id":          state.CheckId,
			"min_failing_count": state.MinFailingCount,
			"min_failing_time":  state.MinFailingTime,
			"failing_count":     state.FailingCount,
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
