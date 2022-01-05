package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/subcommands"
	lru "github.com/hashicorp/golang-lru"
	"github.com/jrhy/s3db"
)

func main() {

	prefix := flag.String("p", "", "prefix of s3db")
	bucket := flag.String("b", "", "s3 bucket containing s3db")
	keyFile := flag.String("k", "", "encryption keyphrase")
	flag.Parse()
	if *prefix == "" || *bucket == "" {
		fmt.Printf("%v %v\n", *prefix, *bucket)
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()
	s := getS3()
	var nodeEncryptor s3db.Encryptor
	if *keyFile != "" {
		keyBytes, err := ioutil.ReadFile(*keyFile)
		if err != nil {
			panic(err)
		}
		nodeEncryptor = s3db.V1NodeEncryptor(keyBytes)
	}
	cfg := s3db.Config{
		Storage: &s3db.S3BucketInfo{
			EndpointURL: s.Endpoint,
			BucketName:  *bucket,
			Prefix:      *prefix,
		},
		KeysLike:      "stringy",
		ValuesLike:    "stringy",
		NodeEncryptor: nodeEncryptor,
	}

	db, err := s3db.Open(ctx, s, cfg,
		s3db.OpenOptions{
			ReadOnly: true,
		},
		time.Now(),
	)
	if err != nil {
		panic(err)
	}

	var runningActiveCases int
	const caseDurationDays = 10
	var oldCases int
	var oldDay string
	activeCasesByDay := make(map[string]int)
	cache, err := lru.NewWithEvict(caseDurationDays, func(key, value interface{}) {
		oldDay = key.(string)
		oldCases = value.(int)
	})
	if err != nil {
		panic(err)
	}
	totalCases := 0
	err = db.Diff(ctx, nil, func(key, myValue, fromValue interface{}) (keepGoing bool, err error) {
		_, err = time.Parse("2006-01-02", key.(string))
		if err != nil {
			return false, fmt.Errorf("bad day: %v: %w", key, err)
		}
		newCases, err := strconv.Atoi(myValue.(string))
		if err != nil {
			return false, fmt.Errorf("bad case count for day %v: %v: %w", key, myValue, err)
		}
		totalCases += newCases
		oldCases = 0
		oldDay = ""
		cache.Add(key.(string), newCases)
		runningActiveCases += newCases - oldCases
		activeCasesByDay[key.(string)] = runningActiveCases

		oldActiveCases := float64(activeCasesByDay[oldDay])
		if oldActiveCases == 0.0 {
			oldActiveCases = float64(runningActiveCases)
		}
		adg := (math.Exp(math.Log(float64(runningActiveCases)/float64(oldActiveCases))/
			float64(caseDurationDays)) - 1.0) * 100.0
		r0 := math.Pow(1.0+adg/100.0, float64(caseDurationDays))
		daysToDoubling := math.Log(2.0) / math.Log(1.0+adg/100.0)
		daysLeftInDiet := math.Log(1250000.0/float64(totalCases)) / math.Log(1.0+adg/100.0)
		fmt.Printf("%v +%d -%d: %d, adg %.1f%%, r0=%.1f 2x/%.1fd daysLeftInDiet:%d\n", key, newCases, oldCases, runningActiveCases, adg, r0, daysToDoubling, int(daysLeftInDiet+0.5))
		return true, nil
	})
	if err != nil {
		panic(err)
	}

}

func getS3() *s3.S3 {
	config := aws.Config{}
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint != "" {
		config.Endpoint = &endpoint
		config.S3ForcePathStyle = aws.Bool(true)
	}

	sess, err := session.NewSession(&config)
	if err != nil {
		err = fmt.Errorf("session: %w", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(int(subcommands.ExitFailure))
	}

	return s3.New(sess)
}
