/* There are two hot tables in DynamoDB for CheckResults and CheckResponses
named check_results and check_responses respectively.

Querying check_results is generally done by querying one of the two
Global Secondary Indexes (GSIs). The first GSI is on check_id, and
the second is on customer_id. Querying GSI will return tuples of
(check_id, result_id) and (customer_id, result_id) items respectively.

The partition key for check_results, "result_id" is a combination of
<check_id>:<bastion_id>. This should allow relatively even
partitioning of the checks.

Examples:

To get results for a single check, you would execute the query:

{
    "TableName": "check_results",
    "IndexName": "check_id-index",
    "KeyConditionExpression": "check_id = :check_id"
}

You would then need to execute a BatchGetItem request to get
all of the result objects at once.

TODO: we should cache the responses from the first query, because those
won't change very often. The response from the second request will
probably never be worth caching.

To get all results for a customer, you would execute the query:

{
    "TableName": "check_results",
    "IndexName": "customer_id-index",
    "KeyConditionExpression": "customer_id = :customer_id"
}

Similarly, you would then follow it up with a BatchGetItem request for
every result in the query result set.

CheckResponses are indexed by a "response_id" which is the combination
<check_id>:<bastion_id>:<target_id>. To get the responses associated
with a CheckResult, you first query check_results. The result returned
will include a "responses" field that will be an array of string values
that are the response_ids of the associated responses. You can then issue
a BatchGetItem request on check_responses to get each of those.
*/
package results

import (
	"fmt"

	log "github.com/opsee/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/gogo/protobuf/proto"
	"github.com/opsee/basic/schema"
	opsee_types "github.com/opsee/protobuf/opseeproto/types"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	CheckResultTableName           = "check_results"
	CheckResultCheckIdIndexName    = "check_id-index"
	CheckResultCustomerIdIndexName = "customer_id-index"
	CheckResponseTableName         = "check_responses"
)

var (
	checkResultsTablePutItem = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "check_results_put_items",
		Help: "Total number of PutItem calls on the check_results table.",
	})

	checkResponsesTablePutItem = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "check_responses_put_items",
		Help: "Total number of PutItem calls on the check_responses table.",
	})
)

func init() {
	prometheus.MustRegister(checkResultsTablePutItem)
	prometheus.MustRegister(checkResponsesTablePutItem)
}

/*
message CheckResult {
	string check_id = 1;
	string customer_id = 2;
	opsee.types.Timestamp timestamp = 3;
	bool passing = 4;
	repeated CheckResponse responses = 5;
	Target target = 6;
	string check_name = 7;
	int32 version = 8;
  string bastion_id = 9;
}
*/

/*
  Table: check_results
  Primary Key: check_id
  Sort Key: result_id = <bastion_id>:<timestamp>
*/

type DynamoStore struct {
	DynaClient *dynamodb.DynamoDB
}

func (s *DynamoStore) GetResultsByCheckId(checkId string) ([]*schema.CheckResult, error) {
	logger := log.WithFields(log.Fields{
		"fn":       "GetResultsByCheckId",
		"check_id": checkId,
	})
	checkIdAv, err := dynamodbattribute.Marshal(checkId)
	if err != nil {
		return nil, err
	}

	// First we must query check_results-index for the result_ids for that check.
	params := &dynamodb.QueryInput{
		TableName:              aws.String(CheckResultTableName),
		IndexName:              aws.String(CheckResultCheckIdIndexName),
		KeyConditionExpression: aws.String("check_id = :check_id"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":check_id": checkIdAv,
		},
	}

	checkIndexResponse, err := s.DynaClient.Query(params)
	if err != nil {
		logger.WithError(err).Error("Error querying dynamodb check index.")
		return nil, err
	}

	results := make([]*schema.CheckResult, len(checkIndexResponse.Items))
	for i, resultAvMap := range checkIndexResponse.Items {
		logger := log.WithFields(log.Fields{
			"fn":        "CheckResults",
			"check_id":  checkId,
			"result_id": aws.StringValue(resultAvMap["result_id"].S),
		})
		// Now we must call GetItem for that result_id
		resultGetItemResponse, err := s.DynaClient.GetItem(&dynamodb.GetItemInput{
			TableName: aws.String(CheckResultTableName),
			Key: map[string]*dynamodb.AttributeValue{
				"result_id": resultAvMap["result_id"],
			},
		})
		if err != nil {
			logger.WithError(err).Error("Error getting result item from dyanmodb")
			return nil, err
		}

		dynamoCheckResult := resultGetItemResponse.Item
		result := &schema.CheckResult{}
		if err := dynamodbattribute.UnmarshalMap(dynamoCheckResult, result); err != nil {
			logger.WithError(err).Error("Error unmarshalling check result from dynamodb")
			return nil, err
		}

		responseIds := []string{}
		err = dynamodbattribute.Unmarshal(dynamoCheckResult["responses"], &responseIds)
		if err != nil {
			logger.WithError(err).Error("Error unmarshalling response list from dynamodb")
			return nil, err
		}

		checkResponses := make([]*schema.CheckResponse, len(responseIds))
		for j, responseId := range responseIds {
			logger := log.WithFields(log.Fields{
				"fn":          "CheckResults",
				"check_id":    checkId,
				"result_id":   aws.StringValue(resultAvMap["result_id"].S),
				"response_id": responseId,
			})
			responseIdAv, err := dynamodbattribute.Marshal(responseId)

			responseGetItemResponse, err := s.DynaClient.GetItem(&dynamodb.GetItemInput{
				TableName: aws.String(CheckResponseTableName),
				Key: map[string]*dynamodb.AttributeValue{
					"response_id": responseIdAv,
				},
			})
			if err != nil {
				logger.WithError(err).Error("Error getting response item from dynamodb.")
				return nil, err
			}

			checkResponse := &schema.CheckResponse{}
			responseProtoAv, ok := responseGetItemResponse.Item["response_protobuf"]
			if !ok {
				err := fmt.Errorf("Response in dynamodb had no response object.")
				logger.WithError(err).Error("Empty response protobuf in dynamodb.")
				return nil, err
			}
			responseProto := []byte{}
			if err := dynamodbattribute.Unmarshal(responseProtoAv, &responseProto); err != nil {
				logger.WithError(err).Error("Error unmarshalling response protobuf from dynamodb")
				return nil, err
			}
			if err := proto.Unmarshal(responseProto, checkResponse); err != nil {
				logger.WithError(err).Error("Error unmarshalling response protobuf")
				return nil, err
			}
			checkResponses[j] = checkResponse
		}

		result.Responses = checkResponses
		results[i] = result
	}

	return results, nil
}

func (s *DynamoStore) PutResult(result *schema.CheckResult) error {
	var (
		bastionId string
		item      map[string]*dynamodb.AttributeValue
	)

	// If we choose to store replies/responses separately in dynamodb, then
	// we can just add (gogoproto.moretags) = "dynamodbav:\"-\""
	// That will cause dyanmodbattribute.MarshalMap() to ignore them.
	item, err := dynamodbattribute.MarshalMap(result)
	if err != nil {
		return err
	}

	if bid := result.BastionId; bid == "" {
		bastionId = result.CustomerId
	} else {
		bastionId = bid
	}

	resultId := fmt.Sprintf("%s:%s", result.CheckId, bastionId)
	rid, err := dynamodbattribute.Marshal(resultId)
	if err != nil {
		return err
	}
	item["result_id"] = rid

	responseIds := make([]string, len(result.Responses))
	log.WithFields(log.Fields{"result_id": resultId}).Debugf("Result has %d responses.", len(result.Responses))
	// TODO(greg): parallelize these while maintaining the contract that we
	// return an error if we have a problem writing a response to dynamodb so
	// that we requeue and retry.
	for i, r := range result.Responses {
		if r.Reply == nil && r.Response != nil {
			any, err := opsee_types.UnmarshalAny(r.Response)
			if err != nil {
				return err
			}

			switch reply := any.(type) {
			case *schema.HttpResponse:
				r.Reply = &schema.CheckResponse_HttpResponse{reply}
			case *schema.CloudWatchResponse:
				r.Reply = &schema.CheckResponse_CloudwatchResponse{reply}
			}

			if result.Version < 2 {
				if r.Target.Type == "host" || r.Target.Type == "external_host" {
					if r.Target.Address != "" {
						r.Target.Id = r.Target.Address
					}
				}
			}
		}

		responseProto, err := proto.Marshal(r)
		if err != nil {
			return err
		}
		responseProtoAv, err := dynamodbattribute.Marshal(responseProto)
		if err != nil {
			return err
		}

		item := map[string]*dynamodb.AttributeValue{}
		item["response_protobuf"] = responseProtoAv

		responseId := fmt.Sprintf("%s:%s:%s", result.CheckId, result.BastionId, r.Target.Id)
		responseIds[i] = responseId

		responseIdAv, err := dynamodbattribute.Marshal(responseId)
		if err != nil {
			return err
		}
		item["response_id"] = responseIdAv

		log.WithFields(log.Fields{"response_id": responseId}).Debugf("Putting %d of %d responses to DynamoDB.", i+1, len(result.Responses))
		params := &dynamodb.PutItemInput{
			TableName: aws.String(CheckResponseTableName),
			Item:      item,
		}
		_, err = s.DynaClient.PutItem(params)
		if err != nil {
			return err
		}
		checkResponsesTablePutItem.Inc()
	}

	responseIdsAv, err := dynamodbattribute.Marshal(responseIds)
	if err != nil {
		return err
	}

	item["responses"] = responseIdsAv

	params := &dynamodb.PutItemInput{
		TableName: aws.String(CheckResultTableName),
		Item:      item,
	}

	_, err = s.DynaClient.PutItem(params)
	if err != nil {
		return err
	}
	checkResultsTablePutItem.Inc()

	return nil
}
