package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

const (
	totalSegments = 128
	targetWCU     = 5000.0
	targetRCU     = 5000.0
	tableName     = "table"
)

var (
	lk           sync.Mutex
	segmentsLeft int32
	totalRCU     float64
	totalWCU     float64
	totalScanned int64
	totalMatched int64
	totalErrors  int64
	totalDeleted int64
)

func main() {
	ctx := context.Background()
	start := time.Now()
	s := session.New(&aws.Config{
		Region: aws.String("us-west-2"),
	})
	d := dynamodb.New(s)
	l := log.New(os.Stdout, "", 0)

	wg := sync.WaitGroup{}

	segmentsLeft = totalSegments
	wg.Add(1)
	go func() {
		for i := 0; i < totalSegments; i++ {
			segment := i

			wg.Add(1)
			go func() {
				scan(ctx, l, d, segment, totalSegments)
				atomic.AddInt32(&segmentsLeft, -1)
				wg.Done()
			}()

		}
		wg.Done()
	}()

	go func() {
		deletedInterval := int64(0)
		for {
			logWithDetails(l, "progress",
				"items scanned", totalScanned,
				"items matched", totalMatched,
				"items deleted", totalDeleted,
				"d/s", int(float64(totalDeleted-deletedInterval)/5.0),
				"RCU", totalRCU,
				"WCU", totalWCU,
				"errors", totalErrors,
			)
			deletedInterval = totalDeleted
			time.Sleep(5 * time.Second)
		}
	}()

	wg.Wait()
	logWithDetails(l, "deletion rate", "n/s", float64(totalDeleted)/(float64(time.Now().Sub(start).Milliseconds())*1000.0))
}

func scan(ctx context.Context, l *log.Logger, d *dynamodb.DynamoDB, segment, totalSegments int) {
	input := dynamodb.ScanInput{
		TableName:              aws.String(tableName),
		Segment:                aws.Int64(int64(segment)),
		TotalSegments:          aws.Int64(int64(totalSegments)),
		ReturnConsumedCapacity: aws.String(dynamodb.ReturnConsumedCapacityTotal),
		ProjectionExpression:   aws.String("P,S"),
	}

	var n int64
	var cu float64
	last := time.Now()
	for {
		output, err := d.ScanWithContext(ctx, &input)
		if err != nil {
			err = fmt.Errorf("scanWithContext: %v (sleeping)", err)
			time.Sleep(2)
			continue
		}
		waitMyTurn(last, *output.ConsumedCapacity.CapacityUnits)
		atomic.AddInt64(&totalScanned, *output.ScannedCount)
		atomic.AddInt64(&totalMatched, *output.Count)
		lk.Lock()
		totalRCU += *output.ConsumedCapacity.CapacityUnits
		lk.Unlock()
		writeRequests := itemsToDeleteRequests(output.Items)
		for len(writeRequests) > 0 {
			j := len(writeRequests)
			if j > 25 {
				j = 25
			}

			output, wcu := deleteBatch(ctx, l, d, writeRequests[0:j])
			lk.Lock()
			totalWCU += wcu
			lk.Unlock()

			writeRequests = writeRequests[j:]
			if output.UnprocessedItems != nil {
				writeRequests = append(writeRequests, output.UnprocessedItems[tableName]...)
			}

			last = waitMyTurn(last, wcu)
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	logWithDetails(l, "segment scan completed", "segment", segment, "item_count", n, "capacity_consumed", cu)
}

func deleteBatch(ctx context.Context, l *log.Logger, d *dynamodb.DynamoDB, requests []*dynamodb.WriteRequest) (*dynamodb.BatchWriteItemOutput, float64) {
	input := dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]*dynamodb.WriteRequest{
			tableName: requests,
		},
		ReturnConsumedCapacity: aws.String(dynamodb.ReturnConsumedCapacityTotal),
	}
	output, err := d.BatchWriteItemWithContext(ctx, &input)
	if err != nil {
		atomic.AddInt64(&totalErrors, 1)
		fmt.Printf("delete err: %v (sleeping)\n", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil && output == nil {
		panic("you were hoping for output always!")
	}
	totalCap := 0.0
	for i := range output.ConsumedCapacity {
		totalCap += *output.ConsumedCapacity[i].CapacityUnits
	}
	atomic.AddInt64(&totalDeleted, int64(len(requests)-len(output.UnprocessedItems[tableName])))
	return output, totalCap
}

func itemsToDeleteRequests(items []map[string]*dynamodb.AttributeValue) []*dynamodb.WriteRequest {
	requests := []*dynamodb.WriteRequest{}
	for i := 0; i < len(items); i++ {
		requests = append(requests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{
				Key: items[i],
			},
		})
	}
	return requests
}

func waitMyTurn(last time.Time, wcu float64) time.Time {
	d := time.Duration(wcu*float64(atomic.AddInt32(&segmentsLeft, 0))) * time.Second / targetWCU
	next := last.Add(d)
	now := time.Now()
	if next.After(now) {
		time.Sleep(next.Sub(now))
		return next
	}
	return now
}

func logWithDetails(l *log.Logger, s string, args ...interface{}) {
	o := map[string]interface{}{}
	o["message"] = s
	for i := 0; i < len(args)-1; i += 2 {
		k, v := args[i].(string), args[i+1]
		o[k] = v
	}
	os, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	l.Println(string(os))
}
