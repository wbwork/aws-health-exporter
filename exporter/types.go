package exporter

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/directconnect"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/health"
	healthTypes "github.com/aws/aws-sdk-go-v2/service/health/types"
	"github.com/slack-go/slack"
)

type Metrics struct {
	health *health.Client
	ec2    map[string]ec2.Client
	dx     map[string]directconnect.Client

	slackApi     *slack.Client
	slackToken   string
	slackChannel string

	tz         *time.Location
	lastScrape time.Time

	awsconfig           aws.Config
	organizationEnabled bool
	regions             []string

	ignoreEvents        []string
	ignoreResources     []string
	ignoreResourceEvent []string

	accountNames map[string]string

	logEvents bool
}

type HealthEvent struct {
	Arn               *string
	AffectedAccounts  []string
	EventScope        healthTypes.EventScopeCode
	Event             *healthTypes.Event
	EventDescription  *healthTypes.EventDescription
	AffectedResources []healthTypes.AffectedEntity
	Tags              map[string]string
}
