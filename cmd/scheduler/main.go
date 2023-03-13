package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/itsHabib/rsvper/internal/cfa"
	"github.com/itsHabib/rsvper/internal/scheduler"
)

const (
	awsRegion       = "us-east-2"
	requestFilePath = "cmd/scheduler/requests.json"
	credsFilePath   = "cmd/scheduler/creds.txt"
)

var (
	username string
	password string
)

func main() {
	c := &http.Client{
		Timeout: 10 * time.Second,
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	cfaService, err := cfa.NewService(c)
	if err != nil {
		log.Fatalf("unable to create cfa service: %v", err)
	}
	sess, err := getAWSSession()
	if err != nil {
		log.Fatalf("unable to get aws session: %v", err)
	}
	schedulerService, err := scheduler.NewService(sess)
	if err != nil {
		log.Fatalf("unable to create scheduler service: %v", err)
	}

	// get requests file
	requestsFile, err := os.Open(requestFilePath)
	if err != nil {
		log.Fatalf("unable to open requests file: %v", err)
	}
	var requests []cfa.ScheduleRequest
	if err := json.NewDecoder(requestsFile).Decode(&requests); err != nil {
		log.Fatalf("unable to decode requests file: %v", err)
	}
	if len(requests) == 0 {
		fmt.Printf("no requests to process\n")
		return
	}
	fmt.Printf("loaded %d requests\n", len(requests))
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].StartTime.Before(*requests[j].StartTime)
	})

	// login to set cookie
	if err := readCreds(); err != nil {
		log.Fatalf("unable to read creds: %v", err)
	}
	cookie, err := cfaService.Login(username, password)
	if err != nil {
		log.Fatalf("unable to login: %v", err)
	}
	fmt.Println("successfully logged in")

	// form get schedule params
	start := fmt.Sprintf(
		"%d-%02d-%02d",
		requests[0].StartTime.Year(),
		requests[0].StartTime.Month(),
		requests[0].StartTime.Day(),
	)
	end := fmt.Sprintf(
		"%d-%02d-%02d",
		requests[len(requests)-1].StartTime.Year(),
		requests[len(requests)-1].StartTime.Month(),
		requests[len(requests)-1].StartTime.Day(),
	)
	params := cfa.ScheduleParams{
		Name:      cfa.InHouseSessions,
		StartDate: start,
		EndDate:   end,
	}
	// get schedule
	schedule, err := cfaService.GetSchedule(params)
	if err != nil {
		log.Fatalf("unable to get schedule: %v", err)
	}

	// process requests and schedules to create scheduled events
	if err := schedulerService.ProcessRequests(*cookie, schedule, requests); err != nil {
		log.Fatalf("unable to process requests: %v", err)
	}
}

func getAWSSession() (*session.Session, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create new session: %w", err)
	}

	return sess, nil
}

func readCreds() error {
	f, err := os.Open(credsFilePath)
	if err != nil {
		return fmt.Errorf("unable to open creds file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		username = scanner.Text()
	}
	if scanner.Scan() {
		password = scanner.Text()
	}

	if username == "" || password == "" {
		return fmt.Errorf("unable to read username or password")
	}

	return nil
}
