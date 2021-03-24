Here are functions that were going to be used to implement an in-memory
fake DynamoDB server in Go, a lighter alternative to using DynamoDBLocal.
If you end up using them, please let me know!  mailto:fromtheweb@watson.rhyason.com

Status
======
Implemented: Expression parser and evaluator that understand DynamoDB ConditionExpressions.  `ddbexpr_test.go` is the best source of information.

Future Directions
=================
* UpdateExpressions
* Property testing


