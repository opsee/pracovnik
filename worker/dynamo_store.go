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
package worker

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/opsee/basic/schema"
	opsee_types "github.com/opsee/protobuf/opseeproto/types"
)

const (
	CheckResultTableName   = "check_results"
	CheckResponseTableName = "check_responses"
)

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

func (s *DynamoStore) GetResults(result *schema.CheckResult) (map[string]*schema.CheckResult, error) {
	checkId := result.CheckId
	customerId := result.CustomerId
	bastionId := result.BastionId
	if bastionId == "" {
		bastionId = customerId
	}

	params := &dynamodb.QueryInput{
		TableName:              aws.String(CheckResultTableName),
		KeyConditionExpression: aws.String(fmt.Sprintf("check_id = %s AND begins_with(result_id, %s:)", checkId, bastionId)),
		ScanIndexForward:       aws.Bool(false),
		Select:                 aws.String("ALL_ATTRIBUTES"),
		Limit:                  aws.Int64(1),
	}

	resp, err := s.DynaClient.Query(params)
	if err != nil {
		return nil, err
	}

	results := map[string]*schema.CheckResult{}
	for _, item := range resp.Items {
		resultId := item["result_id"]
		splitResultId := strings.Split(aws.StringValue(resultId.S), ":")
		resultBastionId := splitResultId[0]

		bastionResult := &schema.CheckResult{}
		if err := dynamodbattribute.UnmarshalMap(item, bastionResult); err != nil {
			return nil, err
		}

		params := &dynamodb.QueryInput{
			TableName:              aws.String(CheckResponseTableName),
			KeyConditionExpression: aws.String(fmt.Sprintf("check_id = %s AND result_id = %s", checkId, resultId)),
			Select:                 aws.String("ALL_ATTRIBUTES"),
		}
		grResp, err := s.DynaClient.Query(params)
		if err != nil {
			return nil, err
		}

		responses := make([]*schema.CheckResponse, 0, len(grResp.Items))
		for i, response := range grResp.Items {
			checkResponse := &schema.CheckResponse{}
			if err := dynamodbattribute.UnmarshalMap(response, checkResponse); err != nil {
				return nil, err
			}
			responses[i] = checkResponse
		}
		bastionResult.Responses = responses
		results[resultBastionId] = bastionResult
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

	resultId := fmt.Sprintf("%s:%d", result.CheckId, bastionId)
	rid, err := dynamodbattribute.Marshal(resultId)
	if err != nil {
		return err
	}
	item["result_id"] = rid

	responseIds := make([]string, 0, len(result.Responses))

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

		item, err := dynamodbattribute.MarshalMap(r)
		if err != nil {
			return err
		}

		responseId := fmt.Sprintf("%s:%s:%s", result.CheckId, result.BastionId, r.Target.Id)
		responseIds[i] = responseId

		responseIdAv, err := dynamodbattribute.Marshal(responseId)
		if err != nil {
			return err
		}
		item["response_id"] = responseIdAv

		params := &dynamodb.PutItemInput{
			TableName: aws.String(CheckResponseTableName),
			Item:      item,
		}
		_, err = s.DynaClient.PutItem(params)
		if err != nil {
			return err
		}
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

	return nil
}
