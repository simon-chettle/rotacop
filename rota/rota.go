package rota

import (
	"errors"
	"fmt"
	"time"

	duration "github.com/ChannelMeter/iso8601duration"
	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/nlopes/slack"
	"github.com/robfig/cron"
)

type Rota struct {
	ID           string
	Name         string
	Duration     string
	Participants map[int]string
	Alert        Alert
}

type Alert struct {
	CronExpression string
	Message        string
}

type History struct {
	ID            string
	RotaID        string
	EndDateTime   int64
	SlackUsername string
}

type RotaHelper struct {
	Rtm         *slack.RTM
	dynamodbSvc *dynamodb.DynamoDB
}

func NewRotaHelper(rtm *slack.RTM) RotaHelper {
	sess, _ := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Credentials: credentials.NewSharedCredentials("/Users/simonchettle/.aws/credentials", "default"),
	})
	svc := dynamodb.New(sess)

	return RotaHelper{rtm, svc}
}

func (r *RotaHelper) usernameToID(username string) string {
	users, _ := r.Rtm.GetUsers()
	for _, v := range users {
		if v.Name == username {
			return v.ID
		}
	}
	return "unknown"
}

func (r *RotaHelper) ChannelNameToID(name string) string {
	channels, _ := r.Rtm.GetChannels(true)
	for _, v := range channels {
		if v.Name == name {
			return v.ID
		}
	}
	return "unknown"
}

func (r *RotaHelper) GetUserOnDuty(rotaID string, convertToID bool) string {

	fmt.Println("Looking for: " + rotaID)

	// get the latest one from the database
	history := r.GetRotaHistory(rotaID)

	// check if the latest one is still on duty, if not pick the next one
	// t, _ := time.Parse("2006-01-02T15:04:05.000Z", history.EndDateTime)

	if history.EndDateTime > time.Now().Unix() {
		if convertToID {
			return r.usernameToID(history.SlackUsername)
		}
		return history.SlackUsername
	}

	rota, _ := r.GetRota(rotaID)

	// We know the current user from the history, so just get the next participant in line (or the first)
	position := 0
	for k, v := range rota.Participants {
		if v == history.SlackUsername {
			position = k
			break
		}
	}

	var onDuty string
	if position == 0 || position == len(rota.Participants) {
		onDuty = rota.Participants[1]
	} else {
		onDuty = rota.Participants[position+1]
	}

	// create a new history record for this new onDuty user
	r.InsertHistory(rota, onDuty)

	if convertToID {
		return r.usernameToID(onDuty)
	}
	return onDuty
}

func (r *RotaHelper) GetRotaHistory(rotaID string) History {

	input := &dynamodb.ScanInput{
		TableName: aws.String("rota_history"),
	}

	result, err := r.dynamodbSvc.Scan(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				fmt.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				fmt.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				fmt.Println(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		panic("eeeek")
	}

	// no sorting in Dynamo, so do a quick loop to get the latest :see_no_evil:
	historyObj := []History{}
	latestHistory := History{}
	dynamodbattribute.UnmarshalListOfMaps(result.Items, &historyObj)
	var latest int64

	for _, v := range historyObj {
		if rotaID == v.RotaID && v.EndDateTime > latest {
			latest = v.EndDateTime
			latestHistory = v
		}
	}

	return latestHistory
}

func (r *RotaHelper) InsertHistory(rota Rota, slackUsername string) bool {

	duration, err := duration.FromString(rota.Duration)
	endDate := time.Now().Add(duration.ToDuration())
	history := History{uuid.New().String(), rota.ID, endDate.Unix(), slackUsername}

	av, err := dynamodbattribute.MarshalMap(history)
	if err != nil {
		panic(fmt.Sprintf("failed to DynamoDB marshal Record, %v", err))
	}

	_, err = r.dynamodbSvc.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("rota_history"),
		Item:      av,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to put Record to DynamoDB, %v", err))
	}

	return true
}

func (r *RotaHelper) GetRotas() []Rota {
	return []Rota{
		Rota{
			"RC",
			"Release Coordinator",
			"P0Y0DT0H0M10S",
			map[int]string{1: "sc", 2: "unknown", 3: "sc", 4: "unknown"},
			Alert{"@every 10s", "You are RC today, please make sure staging is deployed and tested: https://docs.google.com/document/d/13hnr5mVy3-jlG2MnWNp5eBRAXxrl9CkKsXy7J0Wwm4w/edit"},
		},
		// Rota{
		// 	"BH",
		// 	"Bug Hat",
		// 	"P0Y0DT0H0M15S",
		// 	map[int]string{1: "sc", 2: "unknown", 3: "sc", 4: "unknown"},
		// 	Alert{"@every 15s", "You are Bug Hat today: https://docs.google.com/document/d/1SUt-JUyV8d5Nc6c-xjLwUlmH7ZpLxQ3w--XzLb92lDQ/edit"},
		// },
	}
}

func (r *RotaHelper) GetRota(id string) (rota Rota, err error) {
	for _, v := range r.GetRotas() {
		if v.ID == id {
			return v, err
		}
	}

	return rota, errors.New("Rota not found")
}

// Monitor for crons
func (r *RotaHelper) Monitor(rotas []Rota) {
	c := cron.New()

	for _, rota := range rotas {

		fmt.Println("Adding monitor for: " + rota.ID)
		err := c.AddFunc(rota.Alert.CronExpression, func() {
			fmt.Println("Running monitor for: " + rota.ID)
			fmt.Println(rota.Alert.Message)
			prefix := fmt.Sprintf("<@%s> ", r.GetUserOnDuty(""+rota.ID, true))
			r.Rtm.SendMessage(r.Rtm.NewOutgoingMessage(prefix+rota.Alert.Message, r.ChannelNameToID("schettletest")))
		})
		if err != nil {
			r.Rtm.SendMessage(r.Rtm.NewOutgoingMessage("Failed to set monitor: "+err.Error(), r.ChannelNameToID("schettletest")))
		}

	}

	go c.Start()
}
