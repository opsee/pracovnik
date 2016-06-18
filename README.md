# Pracovnik

You know: the "Czech 'worker'".

## Worker

Pracovnik consumes CheckResults from NSQ, manages the state of the Check with which
they are associated, and maintains the check state in Postgres and stores CheckResults
and CheckResponses in DynamoDB.

### Configuration

Environment variables needed:

```
- PRACOVNIK_MAX_TASKS - Maximum concurrency for the worker (e.g. 10)
- PRACOVNIK_LOOKUPD_ADDRESSES - space-delimited list of nsqlookupd addresses (e.g. nsqlookupd:4161)
- PRACOVNIK_POSTGRES_CONN - URL to postgres connection (e.g. postgres://localhost:5432/hugs)
- PRACOVNIK_ETCD_ADDRESS - etcd api address (e.g. http://localhost:2379)
- PRACOVNIK_ALERTS_SQS_URL - URL to SQS queue for alerting (e.g. https://sqs.us-west-2.amazonaws.com/933693344490/OpseeAlerts)
```

### Postgres and Migrations

Pracovnik piggy-backs on Bartnet's DB. Migrations for Pracovnik are in the Bartnet
project for now. Migrations for this database will probably have to be centrally
managed in the repository of the service that maintains until pracovnik no longer
directly accesses the shared Postgres instance.

Pracovnik _should_ only read from Bartnet's tables, and Bartnet _should_ only read
from Pracovnik's check\_state table.

## Check State Machine

[check states](check_state_machine.jpg)

## State Transition Hooks

TODO: add state transition hooks so that we can get rid of the notification/alert
management portion of beavis.

Alert management will be taken over by check state management and state transition
hooks here. We just need to start publishing notifications to hugs when we transition
to `FAIL` from `FAIL_WAIT` or to `OK` from `PASS_WAIT`.