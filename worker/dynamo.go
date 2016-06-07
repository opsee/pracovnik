package worker

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/opsee/basic/schema"
)

const (
	CheckResultTableName   = "check_results"
	CheckResponseTableName = "check_responses"
)

var (
	// Why is this required?
	dynaClient = dynamodb.New(session.New())
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

func PutResult(result *schema.CheckResult) error {
	var (
		bastionId string
		item      map[string]*dynamodb.AttributeValue
	)

	responses := result.Responses
	result.Responses = nil

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

	resultId := fmt.Sprintf("%s:%d", bastionId, result.Timestamp.Millis())
	rid, err := dynamodbattribute.Marshal(resultId)
	if err != nil {
		return err
	}
	item["result_id"] = rid

	checkIdAv, err := dynamodbattribute.Marshal(result.CheckId)
	if err != nil {
		return err
	}

	// TODO(greg): parallelize these while maintaining the contract that we
	// return an error if we have a problem writing a response to dynamodb so
	// that we requeue and retry.
	for _, r := range responses {
		item, err := dynamodbattribute.MarshalMap(r)
		if err != nil {
			return err
		}
		item["check_id"] = checkIdAv
		item["result_id"] = rid

		params := &dynamodb.PutItemInput{
			TableName: aws.String(CheckResponseTableName),
			Item:      item,
		}
		_, err = dynaClient.PutItem(params)
		if err != nil {
			return err
		}
	}

	params := &dynamodb.PutItemInput{
		TableName: aws.String(CheckResultTableName),
		Item:      item,
	}

	_, err = dynaClient.PutItem(params)
	if err != nil {
		fmt.Println("problem item: ", item)
		return err
	}

	return nil
}
